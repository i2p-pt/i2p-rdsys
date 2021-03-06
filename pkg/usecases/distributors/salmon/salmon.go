// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements the core logic of the Salmon proxy distribution system.
// The theory behind Salmon is presented in the following PETS'16 paper:
// https://censorbib.nymity.ch/#Douglas2016a
// Note that this file does *not* implement any user-facing code.
package salmon

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/delivery"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/delivery/mechanisms"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

const (
	DistName = "salmon"
	// The Salmon paper calls this threshold "T".  Simulation results suggest T
	// = 1/3: <https://censorbib.nymity.ch/pdf/Douglas2016a.pdf#page=7>
	MaxSuspicion         = 0.333
	SalmonTickerInterval = time.Hour * 24
	// Number of bytes.
	InvitationTokenLength = 20
	InvitationTokenExpiry = time.Hour * 24 * 7
	NumProxiesPerUser     = 3 // TODO: This should be configurable.
	TokenCacheFile        = "token-cache.bin"
	UsersFile             = "users.bin"
)

// SalmonDistributor contains all the context that the distributor needs to
// run.
type SalmonDistributor struct {
	ipc      delivery.Mechanism
	cfg      *internal.Config
	wg       sync.WaitGroup
	shutdown chan bool

	TokenCache        map[string]*TokenMetaInfo
	tokenCacheMutex   sync.Mutex
	Users             map[string]*User
	AssignedProxies   core.ResourceMap
	UnassignedProxies core.ResourceMap
	// Assignments keep track of our proxy-to-user mappings.
	Assignments *ProxyAssignments
}

// Trust represents the level of trust we have for a user or proxy.
type Trust int

// TokenMetaInfo represents meta information that's associated with an
// invitation token.  In particular, we keep track of when an invitation token
// was issued and who issued the token.
type TokenMetaInfo struct {
	SecretInviterId string
	IssueTime       time.Time
}

// NewSalmonDistributor allocates and returns a new distributor object.
func NewSalmonDistributor() *SalmonDistributor {
	salmon := &SalmonDistributor{}
	salmon.TokenCache = make(map[string]*TokenMetaInfo)
	salmon.Users = make(map[string]*User)
	salmon.AssignedProxies = make(core.ResourceMap)
	salmon.UnassignedProxies = make(core.ResourceMap)
	salmon.cfg = &internal.Config{}
	salmon.Assignments = NewProxyAssignments()
	return salmon
}

// String implements the Stringer interface.
func (s *SalmonDistributor) String() string {
	return fmt.Sprintf("token cache=%d; users=%d; assigned=%d; unassigned=%d; user2proxy=%d; proxy2user=%d",
		len(s.TokenCache),
		len(s.Users),
		len(s.AssignedProxies),
		len(s.UnassignedProxies),
		len(s.Assignments.UserToProxy),
		len(s.Assignments.ProxyToUser))
}

// addUser adds a new user to Salmon and sets its trust and inviter to the
// provided variables.
func (s *SalmonDistributor) addUser(trust Trust, inviter *User) (*User, error) {

	u, err := NewUser()
	if err != nil {
		return nil, err
	}
	u.InvitedBy = inviter
	u.Trust = trust

	s.Users[u.SecretId] = u
	log.Printf("Created new user with secret ID %q.", u.SecretId)

	return u, nil
}

// convertToProxies converts the Resource elements in the given ResourceDiff to
// Proxy elements, which extend Resources.
func convertToProxies(diff *core.ResourceDiff) {

	convert := func(m core.ResourceMap) {
		for _, rQueue := range m {
			for i, r := range rQueue {
				rQueue[i] = &Proxy{Resource: r}
			}
		}
	}
	convert(diff.New)
	convert(diff.Changed)
	convert(diff.Gone)
}

// processDiff takes as input a resource diff and feeds it into Salmon's
// existing set of resources.
// TODO: How should we handle new proxies that are blocked already?
func (s *SalmonDistributor) processDiff(diff *core.ResourceDiff) {

	convertToProxies(diff)
	for rType, rQueue := range diff.Changed {
		for _, r1 := range rQueue {
			// Is the given resource blocked in a new place?
			q, exists := s.AssignedProxies[rType]
			if !exists {
				continue
			}
			r2, err := q.Search(r1.Uid())
			if err == nil {
				if r1.BlockedIn().HasLocationsNotIn(r2.BlockedIn()) {
					r2.(*Proxy).SetBlocked(s.Assignments)
				}
			}
		}
	}
	// Remove proxies that are now gone.
	for _, rQueue := range diff.Gone {
		for _, r := range rQueue {
			s.Assignments.RemoveProxy(r.(*Proxy))
		}
	}

	s.UnassignedProxies.ApplyDiff(diff)
	// New proxies only belong in UnassignedProxies.
	diff.New = nil
	s.AssignedProxies.ApplyDiff(diff)
	log.Printf("Unassigned proxies: %s; assigned proxies: %s",
		s.UnassignedProxies, s.AssignedProxies)
}

// Init initialises the given Salmon distributor.
func (s *SalmonDistributor) Init(cfg *internal.Config) {
	log.Printf("Initialising %s distributor.", DistName)

	s.addUser(UntouchableTrustLevel, nil)
	s.cfg = cfg
	s.shutdown = make(chan bool)

	log.Printf("Initialising resource stream.")
	s.ipc = mechanisms.NewHttpsIpc(
		"http://"+cfg.Backend.WebApi.ApiAddress+cfg.Backend.ResourceStreamEndpoint,
		"GET",
		s.cfg.Backend.ApiTokens[DistName])
	rStream := make(chan *core.ResourceDiff)
	req := core.ResourceRequest{
		RequestOrigin: DistName,
		ResourceTypes: s.cfg.Distributors.Salmon.Resources,
		Receiver:      rStream,
	}
	s.ipc.StartStream(&req)

	s.wg.Add(1)
	go s.housekeeping(rStream)

	s.tokenCacheMutex.Lock()
	defer s.tokenCacheMutex.Unlock()
	err := internal.Deserialise(cfg.Distributors.Salmon.WorkingDir+TokenCacheFile, &s.TokenCache)
	if err != nil {
		log.Printf("Warning: Failed to deserialise token cache: %s", err)
	}
}

// Shutdown shuts down the given Salmon distributor.
func (s *SalmonDistributor) Shutdown() {

	// Write our token cache to disk so it can persist across restarts.
	s.tokenCacheMutex.Lock()
	defer s.tokenCacheMutex.Unlock()
	err := internal.Serialise(s.cfg.Distributors.Salmon.WorkingDir+TokenCacheFile, s.TokenCache)
	if err != nil {
		log.Printf("Warning: Failed to serialise token cache: %s", err)
	}

	// Signal to housekeeping that it's time to stop.
	close(s.shutdown)
	s.wg.Wait()
}

// Don't call this function directly.  Call findProxies instead.
func (s *SalmonDistributor) findAssignedProxies(inviter *User) []core.Resource {

	var proxies []core.Resource

	// Do the given user's proxies have any free slots?
	inviterProxies := s.Assignments.GetProxies(inviter)
	if len(inviterProxies) == 0 {
		log.Printf("Inviter %q has no assigned proxies.", inviter.SecretId)
	}
	for _, proxy := range inviterProxies {
		if proxy.(*Proxy).IsDepleted(s.Assignments) {
			continue
		}
		proxies = append(proxies, proxy)
		if len(proxies) >= NumProxiesPerUser {
			return proxies
		}
	}

	// If we don't have enough proxies yet, we are going to recursively
	// traverse invitation tree to find already-assigned, non-depleted proxies.
	for _, invitee := range inviter.Invited {
		ps := s.findAssignedProxies(invitee)
		proxies = append(proxies, ps...)
		if len(proxies) >= NumProxiesPerUser {
			return proxies[:NumProxiesPerUser]
		}
	}

	return proxies
}

func (s *SalmonDistributor) findProxies(invitee *User, rType string) []core.Resource {

	if invitee == nil {
		return nil
	}

	var proxies []core.Resource
	// People who registered and admin friends don't have an inviter.
	if invitee.InvitedBy != nil {
		proxies := s.findAssignedProxies(invitee.InvitedBy)
		if len(proxies) == NumProxiesPerUser {
			log.Printf("Returning %d proxies to user.", len(proxies))
			return proxies
		}
	}

	// Take some of our unassigned proxies and allocate them for the given user
	// graph, T(u).
	numRemaining := NumProxiesPerUser - len(proxies)
	if len(s.UnassignedProxies[rType]) < numRemaining {
		numRemaining = len(s.UnassignedProxies[rType])
	}
	newProxies := s.UnassignedProxies[rType][:numRemaining]
	s.UnassignedProxies[rType] = s.UnassignedProxies[rType][numRemaining:]
	log.Printf("Not enough assigned proxies; allocated %d unassigned proxies, %d remaining",
		len(newProxies), len(s.UnassignedProxies))

	for _, p := range newProxies {
		s.AssignedProxies[rType] = append(s.AssignedProxies[rType], p)
		s.Assignments.Add(invitee, p.(*Proxy))
		proxies = append(proxies, p)
	}

	return proxies
}

// GetProxies attempts to return proxies for the given user.
func (s *SalmonDistributor) GetProxies(secretId string, rType string) ([]core.Resource, error) {

	user, exists := s.Users[secretId]
	if !exists {
		return nil, errors.New("user ID does not exists")
	}

	if _, exists := resources.ResourceMap[rType]; !exists {
		return nil, errors.New("requested resource type does not exist")
	}

	// Is Salmon handing out the resources that is requested?
	isSupported := false
	for _, supportedType := range s.cfg.Distributors.Salmon.Resources {
		if rType == supportedType {
			isSupported = true
		}
	}
	if !isSupported {
		return nil, errors.New("requested resource type not supported")
	}

	if user.Banned {
		return nil, errors.New("user is blocked and therefore unable to get proxies")
	}

	// Does the user already have assigned proxies?
	userProxies := s.Assignments.GetProxies(user)
	if len(userProxies) > 0 {
		return userProxies, nil
	}

	return s.findProxies(user, rType), nil
}

// housekeeping keeps track of periodic tasks.
func (s *SalmonDistributor) housekeeping(rStream chan *core.ResourceDiff) {

	defer s.wg.Done()
	defer close(rStream)
	defer s.ipc.StopStream()
	ticker := time.NewTicker(SalmonTickerInterval)
	defer ticker.Stop()

	for {
		select {
		case diff := <-rStream:
			s.processDiff(diff)
		case <-s.shutdown:
			log.Printf("Shutting down housekeeping.")
			return
		case <-ticker.C:
			// Iterate over all users and proxies and update their trust levels if
			// necessary.
			log.Printf("Updating trust levels of %d users.", len(s.Users))
			for _, user := range s.Users {
				user.UpdateTrust()
			}
			log.Printf("Updating trust levels of %d proxies.", len(s.AssignedProxies))
			for _, proxies := range s.AssignedProxies {
				for _, proxy := range proxies {
					proxy.(*Proxy).UpdateTrust(s.Assignments)
				}
			}
			log.Printf("Pruning token cache.")
			s.pruneTokenCache()
		}
	}
}

// pruneTokenCache removes expired tokens from our token cache.
func (s *SalmonDistributor) pruneTokenCache() {

	s.tokenCacheMutex.Lock()
	defer s.tokenCacheMutex.Unlock()

	prevLen := len(s.TokenCache)
	for token, metaInfo := range s.TokenCache {
		if time.Since(metaInfo.IssueTime) > InvitationTokenExpiry {
			// Time to delete the token.
			log.Printf("Deleting expired token %q issued by user %q.", token, metaInfo.SecretInviterId)
			delete(s.TokenCache, token)
		}
	}
	log.Printf("Pruned token cache from %d to %d entries.", prevLen, len(s.TokenCache))
}

// CreateInvite returns an invitation token if the given user is allowed to
// issue invites, and an error otherwise.
func (s *SalmonDistributor) CreateInvite(secretId string) (string, error) {

	u, exists := s.Users[secretId]
	if !exists {
		return "", errors.New("user ID does not exists")
	}

	if u.Banned {
		return "", errors.New("user is blocked and therefore unable to issue invites")
	}

	if u.Trust < MaxTrustLevel {
		return "", errors.New("user's trust level not high enough to issue invites")
	}

	s.tokenCacheMutex.Lock()
	defer s.tokenCacheMutex.Unlock()

	var token string
	var err error
	for {
		token, err = internal.GetRandBase32(InvitationTokenLength)
		if err != nil {
			return "", err
		}

		if _, exists := s.TokenCache[token]; !exists {
			break
		} else {
			// In the highly unlikely case of a token collision, we simply try
			// again.
			log.Printf("Newly created token already exists.  Trying again.")
		}
	}
	log.Printf("User %q issued new invite token %q.", u.SecretId, token)

	// Add token to our token cache, where it remains until it's redeemed or
	// until it expires.
	s.TokenCache[token] = &TokenMetaInfo{secretId, time.Now().UTC()}

	return token, nil
}

// RedeemInvite redeems the given token.  If redemption was successful, the
// function returns the new user's secret ID; otherwise an error.
func (s *SalmonDistributor) RedeemInvite(token string) (string, error) {

	s.tokenCacheMutex.Lock()
	defer s.tokenCacheMutex.Unlock()

	metaInfo, exists := s.TokenCache[token]
	if !exists {
		return "", errors.New("invite token does not exist")
	}
	// Remove token from our token cache.
	delete(s.TokenCache, token)

	// Is our token still valid?
	if time.Since(metaInfo.IssueTime) > InvitationTokenExpiry {
		return "", errors.New("invite token already expired")
	}

	inviter, exists := s.Users[metaInfo.SecretInviterId]
	if !exists {
		log.Printf("Bug: could not find valid user for invite token.")
		return "", errors.New("invite token came from non-existing user (this is a bug)")
	}

	u, err := s.addUser(inviter.Trust-1, inviter)
	if err != nil {
		return "", err
	}

	return u.SecretId, nil
}

// Register lets a user sign up for Salmon.
func (s *SalmonDistributor) Register() (string, error) {

	return "", errors.New("registration not yet implemented")
}

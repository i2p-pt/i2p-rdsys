package distributors

import (
	"testing"
	"time"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

func setup() *SalmonDistributor {
	salmon := &SalmonDistributor{}
	p1 := &Proxy{}
	p2 := &Proxy{}
	p3 := &Proxy{}
	p4 := &Proxy{}

	salmon.AssignedProxies = core.ResourceMap{"obfs4": []core.Resource{p1, p2, p3}}
	salmon.UnassignedProxies = core.ResourceMap{"obfs4": []core.Resource{p4}}

	u1 := &User{}
	u2 := &User{}
	u3 := &User{}
	u2.InvitedBy = u1
	u3.InvitedBy = u1

	u1.Proxies = []core.Resource{p1}
	u1.Invited = []*User{u2, u3}
	u3.Proxies = []core.Resource{p2, p3}

	return salmon
}

func TestUpdateUserTrust(t *testing.T) {
	u := &User{}
	u.Trust = -2

	u.LastPromoted = time.Now().UTC()
	u.UpdateTrust()
	if u.Trust != -2 {
		t.Errorf("incorrect user trust level")
	}

	// Ten seconds before midnight means no promotion.
	u.LastPromoted = time.Now().UTC().Add(-time.Hour*24*2 + time.Second*10)
	u.UpdateTrust()
	if u.Trust != -2 {
		t.Errorf("incorrect user trust level: %d", u.Trust)
	}

	// After 2^abs(-2 + 1) days, the user should be promoted to trust level -1.
	u.LastPromoted = time.Now().UTC().Add(-time.Hour*24*2 - time.Second*10)
	u.UpdateTrust()
	if u.Trust != -1 {
		t.Errorf("incorrect user trust level")
	}

	// After 2^abs(-1 + 1) days, the user should be promoted to trust level 0.
	u.LastPromoted = time.Now().UTC().Add(-time.Hour*24 - time.Second*10)
	u.UpdateTrust()
	if u.Trust != 0 {
		t.Errorf("incorrect user trust level")
	}

	// After 2^abs(0 + 1) days, the user should be promoted to trust level 1.
	u.LastPromoted = time.Now().UTC().Add(-time.Hour*24*2 - time.Second*10)
	u.UpdateTrust()
	if u.Trust != 1 {
		t.Errorf("incorrect user trust level")
	}

	// Ten seconds before midnight means no promotion.
	u.LastPromoted = time.Now().UTC().Add(-time.Hour*24*4 + time.Second*10)
	u.UpdateTrust()
	if u.Trust != 1 {
		t.Errorf("incorrect user trust level")
	}

	// After 2^abs(1 + 1) days, the user should be promoted to trust level 2.
	u.LastPromoted = time.Now().UTC().Add(-time.Hour*24*4 - time.Second*10)
	u.UpdateTrust()
	if u.Trust != 2 {
		t.Errorf("incorrect user trust level")
	}
}

func TestUpdateProxyTrust(t *testing.T) {
	p := &Proxy{}
	u1 := &User{}
	u1.Trust = 1
	u2 := &User{}
	u2.Trust = 2
	p.Users = []*User{u1, u2}

	// Proxy's trust level should be identical to minimum trust level of its
	// users.
	p.UpdateTrust()
	if p.Trust != 1 {
		t.Errorf("determined incorrect proxy trust level")
	}

	// When user gets promoted, the proxy's trust level should increase too.
	u1.Trust++
	p.UpdateTrust()
	if p.Trust != 2 {
		t.Errorf("determined incorrect proxy trust level")
	}

	u1.Trust++
	p.UpdateTrust()
	if p.Trust != 2 {
		t.Errorf("determined incorrect proxy trust level")
	}
}

func TestTokenCache(t *testing.T) {
	salmon := NewSalmonDistributor()
	u := &User{SecretId: "foo"}
	salmon.Users[u.SecretId] = u

	// Banned users are not allowed to invite.
	u.Banned = true
	_, err := salmon.CreateInvite(u.SecretId)
	if err == nil {
		t.Errorf("banned users are not allowed to invite")
	}
	u.Banned = false

	// New users are not allowed to invite.
	_, err = salmon.CreateInvite(u.SecretId)
	if err == nil {
		t.Errorf("user should not yet be allowed to issue invites")
	}

	u.Trust = MaxTrustLevel
	token, err := salmon.CreateInvite(u.SecretId)
	if err != nil {
		t.Errorf("failed to create invite token: %s", err)
	}
	if token == "" {
		t.Errorf("got empty invite token")
	}

	// We should now have a new entry in our token cache.
	if len(salmon.TokenCache) != 1 {
		t.Errorf("new token was not cached")
	}

	// We should now be able to successfully redeem our token.
	_, err = salmon.RedeemInvite(token)
	if err != nil {
		t.Errorf("failed to redeem invite: %s", err)
	}

	// Our token cache should be empty again.
	if len(salmon.TokenCache) != 0 {
		t.Errorf("token was not deleted upon successful redemption")
	}

	// It must not be possible to redeem a token twice.
	_, err = salmon.RedeemInvite(token)
	if err == nil {
		t.Errorf("must not be possible to redeem a token twice")
	}

	// It also must not be possible to redeem a token that doesn't exist in the
	// cache.
	_, err = salmon.RedeemInvite("ThisTokenDoesNotExist")
	if err == nil {
		t.Errorf("must not be possible to redeem token that's not cached")
	}

	// Create another invite, which we'll let expire.
	token, err = salmon.CreateInvite(u.SecretId)
	if err != nil {
		t.Errorf("failed to create invite token: %s", err)
	}
	metaInfo, _ := salmon.TokenCache[token]
	now := time.Now().UTC()
	metaInfo.IssueTime = now.Add(-InvitationTokenExpiry - time.Minute)

	// An expired token must not be redeemable.
	_, err = salmon.RedeemInvite(token)
	if err == nil {
		t.Errorf("expired token must not be redeemable")
	}
}

func TestPruneTokenCache(t *testing.T) {
	salmon := NewSalmonDistributor()
	expiredTime := time.Now().UTC().Add(-InvitationTokenExpiry - time.Minute)
	salmon.TokenCache["DummyToken"] = &TokenMetaInfo{"foo", expiredTime}
	if len(salmon.TokenCache) != 1 {
		t.Errorf("failed to add expired token to cache")
	}

	salmon.pruneTokenCache()
	if len(salmon.TokenCache) != 0 {
		t.Errorf("failed to prune token cache")
	}
}

func TestProcessDiff(t *testing.T) {
	salmon := NewSalmonDistributor()

	// Create a user, a proxy, and assign the proxy to the user.
	u, _ := salmon.NewUser(0, "")
	p := &Proxy{Resource: resources.NewTransport()}
	p.Users = append(p.Users, u)

	queue := core.ResourceQueue{p}
	salmon.AssignedProxies[resources.ResourceTypeObfs4] = queue

	diff := core.NewResourceDiff()
	// Create a new copy of the proxy and mark it as blocked.
	pNew := &Proxy{Resource: resources.NewTransport()}
	pNew.SetBlockedIn(core.LocationSet{"no": true})
	diff.Changed = core.ResourceMap{resources.ResourceTypeObfs4: core.ResourceQueue{pNew}}
	salmon.processDiff(diff)

	// User should now have a blocking event.
	if len(u.InnocencePs) == 0 {
		t.Errorf("expected 1 blocking event but got %d", len(u.InnocencePs))
	}
	// User should now also be banned.
	if !u.Banned {
		t.Errorf("failed to ban user")
	}
}

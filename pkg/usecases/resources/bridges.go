// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resources

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"reflect"
	"strings"
	"time"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
)

const (
	ProtoTypeTCP = "tcp"
	ProtoTypeUDP = "udp"

	DistributorMoat        = "moat"
	DistributorHttps       = "https"
	DistributorEmail       = "email"
	DistributorUnallocated = "unallocated"

	BridgeReloadInterval = time.Hour
)

// Type Addr is a wrapper around net.Addr which provides identical Marshalling
// and Unmarshalling behavior to IPAddr but with the net.Addr interface underneath.
// This allows it to be extended with other address types, such as those used by
// networks which describe connections using properties of cryptographic keys.
type Addr struct {
	net.Addr
}

func (a Addr) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a *Addr) UnmarshalJSON(data []byte) error {
	ipAddr := net.ParseIP(a.Addr.String())
	if ipAddr == nil {
		return fmt.Errorf("Invalid Address Format: %s", a.Addr.String())
	}
	return json.Unmarshal(data, &ipAddr)
}

// Invalid checks if is a valid public address
func (a *Addr) Invalid() bool {
	ipAddr := net.ParseIP(a.Addr.String())
	isIpAddr := ipAddr != nil
	if !isIpAddr {
		// if it's not an IP address, it must be a hostname, even if it's a
		// cryptographic one. If it's a cryptographic one, then it will almost
		// always be connected to a socket on the localhost, so we can assume
		// it's valid.
		return false
	}
	return ipAddr.IsUnspecified() || ipAddr.IsPrivate() || ipAddr.IsLoopback() || ipAddr.IsMulticast() || ipAddr.IsLinkLocalUnicast() || ipAddr.IsLinkLocalUnicast()
}

// BridgeBase implements variables and methods that are shared by vanilla and
// pluggable transport bridges.
type BridgeBase struct {
	core.ResourceBase
	Protocol     string      `json:"protocol"`
	Address      Addr        `json:"address"`
	Port         uint16      `json:"port"`
	Fingerprint  string      `json:"fingerprint"`
	ORAddresses  []ORAddress `json:"or-addresses"`
	Distribution string      `json:"distribution"`
	Flags        Flags       `json:"flags"`
}

type ORAddress struct {
	IPVersion uint16 `json:"ip-version"`
	Port      uint16 `json:"port"`
	Address   Addr   `json:"address"`
}

// Flags exposes the bridge flags
type Flags struct {
	Fast    bool `json:"fast"`
	Stable  bool `json:"stable"`
	Running bool `json:"running"`
	Valid   bool `json:"valid"`
}

// Bridge represents a Tor bridge.
type Bridge struct {
	BridgeBase
	FirstSeen  time.Time    `json:"-"`
	LastSeen   time.Time    `json:"-"`
	Transports []*Transport `json:"-"`
	testFunc   TestFunc
}

// IsPublic always returns false because neither vanilla nor pluggable
// transport bridges are public.  (Granted, our default bridges are, but we
// don't distribute them using rdsys.)
func (b *BridgeBase) IsPublic() bool {
	return false
}

// BridgeUid determines a bridge's hash key by first hashing its fingerprint,
// and then calculating a HashKey over a concatenation of the bridge's type and
// its hashed fingerprint.
func (b *BridgeBase) BridgeUid(rType string) core.Hashkey {
	hFingerprint, err := HashFingerprint(b.Fingerprint)
	if err != nil {
		log.Printf("Bug: Error while hashing fingerprint %s.", b.Fingerprint)
		hFingerprint = b.Fingerprint
	}

	return core.NewHashkey(rType + hFingerprint)
}

// Distributor set for this bridge
func (b *BridgeBase) Distributor() string {
	return b.Distribution
}

func (b *BridgeBase) oidString() string {
	return fmt.Sprintf("%s|%v|%v", b.Distribution, b.ORAddresses, b.Flags)
}

// NewBridge allocates and returns a new Bridge object.
func NewBridge() *Bridge {
	b := &Bridge{BridgeBase: BridgeBase{ResourceBase: *core.NewResourceBase()}}
	// A bridge (without pluggable transports) is always running vanilla Tor
	// over TCP.
	b.Protocol = ProtoTypeTCP
	b.SetType(ResourceTypeVanilla)
	return b
}

// AddTransport adds the given transport to the bridge.
func (b *Bridge) AddTransport(t1 *Transport) {
	for _, t2 := range b.Transports {
		if reflect.DeepEqual(t1, t2) {
			// We already have this transport on record.
			return
		}
	}
	b.Transports = append(b.Transports, t1)
}

func (b *Bridge) IsValid() bool {
	return b.Type() != "" && b.Address.String() != "" && b.Port != 0
}

func (b *Bridge) GetBridgeLine() string {
	return strings.TrimSpace(fmt.Sprintf("%s:%d %s", PrintTorAddr(&b.Address), b.Port, b.Fingerprint))
}

func (b *Bridge) Oid() core.Hashkey {
	return core.NewHashkey(b.GetBridgeLine() + "|" + b.BridgeBase.oidString())
}

func (b *Bridge) Uid() core.Hashkey {
	return b.BridgeUid(b.RType)
}

func (b *Bridge) SetTestFunc(f TestFunc) {
	b.testFunc = f
}

func (b *Bridge) Test() {
	if b.testFunc != nil {
		// if this bridge has transports, we want to test each of them
		for _, t := range b.Transports {
			t.Test()
		}
		// if this bridge has no transports, it is a vanilla bridge
		if len(b.Transports) == 0 {
			b.testFunc(b)
		}
	}
}

func (b *Bridge) String() string {
	return b.GetBridgeLine()
}

func (b *Bridge) Expiry() time.Duration {
	// Bridges should upload new descriptors at least every 18 hours:
	// https://gitweb.torproject.org/torspec.git/tree/dir-spec.txt?id=c2a584144330239d6aa032b0acfb8b5ba26719fb#n369
	return time.Duration(time.Hour * 18)
}

func GetTorBridgeTypes() []string {
	return []string{ResourceTypeVanilla, ResourceTypeObfs4}
}

// PrintTorAddr takes as input a *Addr object and if it contains an IPv6
// address, it wraps it in square brackets.  This is necessary because Tor
// expects IPv6 addresses enclosed by square brackets.
func PrintTorAddr(a *Addr) string {
	s := a.String()
	ipAddr := net.ParseIP(s)
	if ipAddr == nil {
		return s
	}
	if v4 := ipAddr.To4(); len(v4) == net.IPv4len {
		return s
	} else {
		return fmt.Sprintf("[%s]", s)
	}
}

// HashFingerprint takes as input a bridge's fingerprint and hashes it using
// SHA-1, as discussed by Tor Metrics:
// https://metrics.torproject.org/onionoo.html#parameters_lookup
func HashFingerprint(fingerprint string) (string, error) {

	fingerprint = strings.TrimSpace(fingerprint)

	rawFingerprint, err := hex.DecodeString(fingerprint)
	if err != nil {
		return "", err
	}

	rawHFingerprint := sha1.Sum(rawFingerprint)
	hFingerprint := hex.EncodeToString(rawHFingerprint[:])
	return strings.ToUpper(hFingerprint), nil
}

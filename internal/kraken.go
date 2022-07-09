// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/NullHypothesis/zoossh"
	"github.com/prometheus/client_golang/prometheus"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

const (
	KrakenTickerInterval  = 30 * time.Minute
	MinTransportWords     = 3
	MinFunctionalFraction = 0.5
	MinRunningFraction    = 0.5
	TransportPrefix       = "transport"
	ExtraInfoPrefix       = "extra-info"
	RecordEndPrefix       = "-----END SIGNATURE-----"
)

var (
	NotEnoughRunningError = errors.New("There is not enough running bridges")
)

func InitKraken(cfg *Config, shutdown chan bool, ready chan bool, bCtx *BackendContext) {
	log.Println("Initialising resource kraken.")
	ticker := time.NewTicker(KrakenTickerInterval)
	defer ticker.Stop()

	rcol := &bCtx.Resources
	testFunc := bCtx.rTestPool.GetTestFunc()
	// Immediately parse bridge descriptor when we're called, and let caller
	// know when we're done.
	reloadBridgeDescriptors(cfg, rcol, testFunc, bCtx.metrics)
	calcTestedResources(bCtx.metrics, rcol)
	ready <- true
	bCtx.metrics.updateDistributors(cfg, rcol)

	for {
		select {
		case <-shutdown:
			log.Printf("Kraken shut down.")
			return
		case <-ticker.C:
			log.Println("Kraken's ticker is ticking.")
			reloadBridgeDescriptors(cfg, rcol, testFunc, bCtx.metrics)
			pruneExpiredResources(bCtx.metrics, rcol)
			calcTestedResources(bCtx.metrics, rcol)
			bCtx.metrics.updateDistributors(cfg, rcol)
			log.Printf("Backend resources: %s", rcol)
		}
	}
}

// calcTestedResources determines the fraction of each resource state per
// resource type and exposes them via Prometheus.  The function can tell us
// that e.g. among all obfs4 bridges, 0.2 are untested, 0.7 are functional, and
// 0.1 are dysfunctional.
func calcTestedResources(metrics *Metrics, rcol *core.BackendResources) {

	// Map our numerical resource states to human-friendly strings.
	toStr := map[int]string{
		core.StateUntested:      "untested",
		core.StateFunctional:    "functional",
		core.StateDysfunctional: "dysfunctional",
	}

	functionalFractionAcc := 0.
	for rName, hashring := range rcol.Collection {
		nums := map[int]int{
			core.StateUntested:      0,
			core.StateFunctional:    0,
			core.StateDysfunctional: 0,
		}
		for _, r := range hashring.GetAll() {
			nums[r.TestResult().State] += 1
		}
		for state, num := range nums {
			frac := float64(num) / float64(hashring.Len())
			metrics.TestedResources.With(prometheus.Labels{"type": rName, "status": toStr[state]}).Set(frac)
			if state == core.StateFunctional {
				functionalFractionAcc += frac
			}
		}
	}

	// Distribute only functional resources if the fraction is high enough
	// The fraction might be low after a restart as many resources will be
	// untested or if there is an issue with bridgestrap.
	functionalFraction := functionalFractionAcc / float64(len(rcol.Collection))
	rcol.OnlyFunctional = functionalFraction >= MinFunctionalFraction
	if rcol.OnlyFunctional {
		metrics.DistributingNonFunctional.Set(0)
	} else {
		metrics.DistributingNonFunctional.Set(1)
	}
}

func pruneExpiredResources(metrics *Metrics, rcol *core.BackendResources) {

	for rName, hashring := range rcol.Collection {
		origLen := hashring.Len()
		prunedResources := hashring.Prune()
		if len(prunedResources) > 0 {
			log.Printf("Pruned %d out of %d resources from %s hashring.", len(prunedResources), origLen, rName)
		}
		metrics.Resources.With(prometheus.Labels{"type": rName}).Set(float64(hashring.Len()))
	}
}

// reloadBridgeDescriptors reloads bridge descriptors from the given
// cached-extrainfo file and its corresponding cached-extrainfo.new.
func reloadBridgeDescriptors(cfg *Config, rcol *core.BackendResources, testFunc resources.TestFunc, metrics *Metrics) {

	//First load bridge descriptors from network status file
	bridges, err := loadBridgesFromNetworkstatus(cfg.Backend.NetworkstatusFile)
	if err != nil {
		if errors.Is(err, NotEnoughRunningError) {
			log.Printf("Ignore the bridges descriptor: %s", err.Error())
			metrics.IgnoringBridgeDescriptors.Set(1)
			return
		}
		log.Printf("Error loading network statuses: %s", err.Error())
	}
	metrics.IgnoringBridgeDescriptors.Set(0)

	distributorNames := make([]string, 0, len(cfg.Backend.DistProportions)+1)
	distributorNames = append(distributorNames, "none")
	for dist := range cfg.Backend.DistProportions {
		distributorNames = append(distributorNames, dist)
	}

	err = getBridgeDistributionRequest(cfg.Backend.DescriptorsFile, distributorNames, bridges)
	if err != nil {
		log.Printf("Error loading bridge descriptors file: %s", err.Error())
	}

	//Update bridges from extrainfo files
	for _, filename := range []string{cfg.Backend.ExtrainfoFile, cfg.Backend.ExtrainfoFile + ".new"} {
		descriptors, err := loadBridgesFromExtrainfo(filename)
		if err != nil {
			log.Printf("Failed to reload bridge descriptors: %s", err)
			continue
		}

		for fingerprint, desc := range descriptors {
			bridge, ok := bridges[fingerprint]
			if !ok {
				log.Printf("Received extrainfo descriptor for bridge %s but could not find bridge with that fingerprint", fingerprint)
				continue
			}
			bridge.Transports = desc.Transports
		}
	}

	bl, err := newBlockList(cfg.Backend.BlocklistFile, cfg.Backend.AllowlistFile)
	if err != nil {
		log.Println("Problem loading block list:", err)
	}

	log.Printf("Adding %d bridges.", len(bridges))
	for _, bridge := range bridges {
		blockedIn := bl.blockedIn(bridge.Fingerprint)

		for _, t := range bridge.Transports {
			if t.Address.Invalid() {
				log.Printf("Reject bridge %s transport %s as its IP is not valid: %s", t.Fingerprint, t.Type(), t.Address.String())
				continue
			}
			t.Flags = bridge.Flags
			t.Distribution = bridge.Distribution
			t.SetBlockedIn(blockedIn)
			t.SetTestFunc(testFunc)
			rcol.Add(t)
		}

		// only hand out vanilla flavour if there are no transports
		if len(bridge.Transports) == 0 {
			if bridge.Address.Invalid() {
				log.Printf("Reject vanilla bridge %s s as its IP is not valid: %s", bridge.Fingerprint, bridge.Address.String())
				continue
			}
			bridge.SetBlockedIn(blockedIn)
			bridge.SetTestFunc(testFunc)
			rcol.Add(bridge)
		}
	}
}

// learn about available bridges by parsing a network status file
func loadBridgesFromNetworkstatus(networkstatusFile string) (map[string]*resources.Bridge, error) {
	bridges := make(map[string]*resources.Bridge)
	consensus, err := zoossh.ParseUnsafeConsensusFile(networkstatusFile)
	if err != nil {
		return nil, err
	}

	runningBridges := 0
	numBridges := 0
	for obj := range consensus.Iterate(nil) {
		status, ok := consensus.Get(obj.GetFingerprint())
		if !ok {
			log.Printf("Could not retrieve network status for bridge %s",
				string(obj.GetFingerprint()))
			continue
		}
		// create a new bridge for this status
		b := resources.NewBridge()
		b.Fingerprint = string(status.GetFingerprint())

		if addr, err := net.ResolveIPAddr("", status.Address.IPv6Address.String()); err == nil {
			b.Address = resources.Addr{addr}
			b.Port = status.Address.IPv6ORPort
			oraddress := resources.ORAddress{
				IPVersion: 6,
				Port:      b.Port,
				Address:   b.Address,
			}
			b.ORAddresses = append(b.ORAddresses, oraddress)
		}
		if addr, err := net.ResolveIPAddr("", status.Address.IPv4Address.String()); err == nil {
			b.Address = resources.Addr{addr}
			b.Port = status.Address.IPv4ORPort
			oraddress := resources.ORAddress{
				IPVersion: 4,
				Port:      b.Port,
				Address:   b.Address,
			}
			b.ORAddresses = append(b.ORAddresses, oraddress)
		}

		b.Flags.Fast = status.Flags.Fast
		b.Flags.Stable = status.Flags.Stable
		b.Flags.Running = status.Flags.Running
		b.Flags.Valid = status.Flags.Valid

		//check to see if the bridge has the running flag
		if status.Flags.Running {
			bridges[b.Fingerprint] = b
			runningBridges++
		} else {
			log.Printf("Found bridge %s in networkstatus but is not running", b.Fingerprint)
		}
		numBridges++
	}

	runningBridgesFraction := float64(runningBridges) / float64(numBridges)
	if runningBridgesFraction < MinFunctionalFraction {
		// Fail if most bridges are marked as non-functional. This happens if bridge authority restarts (#102)
		// XXX: If bridge authority restarts at the same time than rdsys the first update will not get any
		//      bridges, hopefully this will not happend.
		return nil, NotEnoughRunningError
	}
	return bridges, nil
}

// getBridgeDistributionRequest from the bridge-descriptors file
func getBridgeDistributionRequest(descriptorsFile string, distributorNames []string, bridges map[string]*resources.Bridge) error {
	descriptors, err := zoossh.ParseUnsafeDescriptorFile(descriptorsFile)
	if err != nil {
		return err
	}

	for fingerprint, bridge := range bridges {
		descriptor, ok := descriptors.Get(zoossh.Fingerprint(fingerprint))
		if !ok {
			log.Printf("Bridge %s from networkstatus not pressent in the descriptors file %s", fingerprint, descriptorsFile)
			continue
		}

		if descriptor.BridgeDistributionRequest != "any" {
			for _, dist := range distributorNames {
				if dist == descriptor.BridgeDistributionRequest {
					bridge.Distribution = dist
					break
				}
			}
			if bridge.Distribution == "" {
				log.Printf("Bridge %s has an unsupported distribution request: %s", fingerprint, descriptor.BridgeDistributionRequest)
			}
		}
	}
	return nil
}

// loadBridgesFromExtrainfo loads and returns bridges from Serge's extrainfo
// files.
func loadBridgesFromExtrainfo(extrainfoFile string) (map[string]*resources.Bridge, error) {

	file, err := os.Open(extrainfoFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	extra, err := parseExtrainfoDoc(file)
	if err != nil {
		return nil, err
	}

	return extra, nil
}

// parseExtrainfoDoc parses the given extra-info document and returns the
// content as a Bridges object.  Note that the extra-info document format is as
// it's produced by the bridge authority.
func parseExtrainfoDoc(r io.Reader) (map[string]*resources.Bridge, error) {

	bridges := make(map[string]*resources.Bridge)

	scanner := bufio.NewScanner(r)
	b := resources.NewBridge()
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// We're dealing with a new extra-info block, i.e., a new bridge.
		if strings.HasPrefix(line, ExtraInfoPrefix) {
			words := strings.Split(line, " ")
			if len(words) != 3 {
				return nil, errors.New("incorrect number of words in 'extra-info' line")
			}
			b.Fingerprint = words[2]
		}

		// We're dealing with a bridge's transport protocols.  There may be
		// several.
		if strings.HasPrefix(line, TransportPrefix) {
			t := resources.NewTransport()
			t.Fingerprint = b.Fingerprint
			err := populateTransportInfo(line, t)
			if err != nil {
				return nil, err
			}
			b.AddTransport(t)
		}

		// Let's store the bridge when the record ends
		if strings.HasPrefix(line, RecordEndPrefix) {
			bridges[b.Fingerprint] = b
			b = resources.NewBridge()
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return bridges, nil
}

// populateTransportInfo parses the given transport line of the format:
//   "transport" transportname address:port [arglist] NL
// ...and writes it to the given transport object.  See the specification for
// more details on what transport lines look like:
// <https://gitweb.torproject.org/torspec.git/tree/dir-spec.txt?id=2b31c63891a63cc2cad0f0710a45989071b84114#n1234>
func populateTransportInfo(transport string, t *resources.Transport) error {

	if !strings.HasPrefix(transport, TransportPrefix) {
		return errors.New("no 'transport' prefix")
	}

	words := strings.Split(transport, " ")
	if len(words) < MinTransportWords {
		return errors.New("not enough arguments in 'transport' line")
	}
	t.SetType(words[1])

	host, port, err := net.SplitHostPort(words[2])
	if err != nil {
		return err
	}
	addr, err := net.ResolveIPAddr("", host)
	if err != nil {
		return err
	}
	t.Address = resources.Addr{&net.IPAddr{addr.IP, addr.Zone}}
	p, err := strconv.Atoi(port)
	if err != nil {
		return err
	}
	t.Port = uint16(p)

	// We may be dealing with one or more key=value pairs.
	if len(words) > MinTransportWords {
		args := strings.Split(words[3], ",")
		for _, arg := range args {
			kv := strings.Split(arg, "=")
			if len(kv) != 2 {
				return fmt.Errorf("key:value pair in %q not separated by a '='", words[3])
			}
			t.Parameters[kv[0]] = kv[1]
		}
	}

	return nil
}

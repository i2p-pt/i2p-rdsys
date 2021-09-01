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
	KrakenTickerInterval = time.Minute
	MinTransportWords    = 3
	TransportPrefix      = "transport"
	ExtraInfoPrefix      = "extra-info"
)

func InitKraken(cfg *Config, shutdown chan bool, ready chan bool, bCtx *BackendContext) {
	log.Println("Initialising resource kraken.")
	ticker := time.NewTicker(KrakenTickerInterval)
	defer ticker.Stop()

	rcol := &bCtx.Resources
	testFunc := bCtx.rTestPool.GetTestFunc()
	// Immediately parse bridge descriptor when we're called, and let caller
	// know when we're done.
	reloadBridgeDescriptors(cfg.Backend.ExtrainfoFile, cfg.Backend.NetworkstatusFile, rcol, testFunc)
	ready <- true

	for {
		select {
		case <-shutdown:
			log.Printf("Kraken shut down.")
			return
		case <-ticker.C:
			log.Println("Kraken's ticker is ticking.")
			reloadBridgeDescriptors(cfg.Backend.ExtrainfoFile, cfg.Backend.NetworkstatusFile, rcol, testFunc)
			pruneExpiredResources(bCtx.metrics, rcol)
			calcTestedResources(bCtx.metrics, rcol)
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
		}
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
func reloadBridgeDescriptors(extrainfoFile, networkstatusFile string, rcol *core.BackendResources, testFunc resources.TestFunc) {

	//First load bridge descriptors from network status file
	bridges, err := loadBridgesFromNetworkstatus(networkstatusFile)
	if err != nil {
		log.Printf("Error loading network statuses: %s", err.Error())
	}

	//Update bridges from extrainfo files
	for _, filename := range []string{extrainfoFile, extrainfoFile + ".new"} {
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
	log.Printf("Adding %d bridges.", len(bridges))
	for _, bridge := range bridges {
		for _, t := range bridge.Transports {
			t.SetTestFunc(testFunc)
			rcol.Add(t)
		}

		// only hand out vanilla flavour if there are no transports
		if len(bridge.Transports) == 0 {
			bridge.SetTestFunc(testFunc)
			rcol.Add(bridge)
		}
	}
}

// learn about available bridges by parsing a network status file
func loadBridgesFromNetworkstatus(networkstatusFile string) (map[string]*resources.Bridge, error) {
	bridges := make(map[string]*resources.Bridge)
	consensus, err := zoossh.ParseConsensusFile(networkstatusFile)
	if err != nil {
		return nil, err
	}

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
			b.Address = resources.IPAddr{*addr}
			b.Port = status.Address.IPv6ORPort
		} else {
			addr, err := net.ResolveIPAddr("", status.Address.IPv4Address.String())
			if err != nil {
				continue
			}
			b.Address = resources.IPAddr{*addr}
			b.Port = status.Address.IPv4ORPort
		}

		//check to see if the bridge has the running flag
		if status.Flags.Running {
			bridges[b.Fingerprint] = b
		} else {
			log.Printf("Found bridge %s in networkstatus but is not running", b.Fingerprint)
		}
	}
	return bridges, nil
}

// loadBridgesFromExtrainfo loads and returns bridges from Serge's extrainfo
// files.
func loadBridgesFromExtrainfo(extrainfoFile string) (map[string]*resources.Bridge, error) {

	file, err := os.Open(extrainfoFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	extra, err := ParseExtrainfoDoc(file)
	if err != nil {
		return nil, err
	}

	return extra, nil
}

// ParseExtrainfoDoc parses the given extra-info document and returns the
// content as a Bridges object.  Note that the extra-info document format is as
// it's produced by the bridge authority.
func ParseExtrainfoDoc(r io.Reader) (map[string]*resources.Bridge, error) {

	var bridges map[string]*resources.Bridge

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		b := resources.NewBridge()
		// We're dealing with a new extra-info block, i.e., a new bridge.
		if strings.HasPrefix(line, ExtraInfoPrefix) {
			words := strings.Split(line, " ")
			if len(words) != 3 {
				return nil, errors.New("incorrect number of words in 'extra-info' line")
			}
			b.Fingerprint = words[2]
			bridges[b.Fingerprint] = b
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
	t.Address = resources.IPAddr{net.IPAddr{addr.IP, addr.Zone}}
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

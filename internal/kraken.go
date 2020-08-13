package internal

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gitlab.torproject.org/tpo/anti-censorship/ouroboros/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/ouroboros/pkg/delivery"
	"gitlab.torproject.org/tpo/anti-censorship/ouroboros/pkg/delivery/mechanisms"
	"gitlab.torproject.org/tpo/anti-censorship/ouroboros/pkg/usecases/resources"
)

const (
	KrakenTickerInterval = time.Minute
	MinTransportWords    = 3
	TransportPrefix      = "transport"
	ExtraInfoPrefix      = "extra-info"
)

type BridgestrapRequest struct {
	BridgeLine string `json:"bridge_line"`
}

type BridgestrapResponse struct {
	Functional bool    `json:"functional"`
	Error      string  `json:"error,omitempty"`
	Time       float64 `json:"time"`
}

func InitKraken(cfg *Config, shutdown chan bool, ready chan bool, rcol ResourceCollection) {
	log.Println("Initialising resource kraken.")
	ticker := time.NewTicker(KrakenTickerInterval)
	defer ticker.Stop()

	// Immediately parse bridge descriptor when we're called, and let caller
	// know when we're done.
	reloadBridgeDescriptors(cfg.Backend.ExtrainfoFile, rcol)
	ready <- true

	bridgestrapCtx := mechanisms.HttpsIpcContext{}
	bridgestrapCtx.ApiEndpoint = "http://localhost:5000/bridge-state"
	bridgestrapCtx.ApiMethod = http.MethodGet
	rcol.Collection[resources.BridgeTypeObfs4].OnAddFunc = queryBridgestrap(&bridgestrapCtx)

	for {
		select {
		case <-shutdown:
			return
		case <-ticker.C:
			log.Println("Kraken's ticker is ticking.")
			reloadBridgeDescriptors(cfg.Backend.ExtrainfoFile, rcol)
		}
	}
}

func queryBridgestrap(m delivery.Mechanism) core.OnAddFunc {

	return func(r core.Resource) {
		log.Printf("Making bridgestrap request.")
		req := BridgestrapRequest{r.String()}
		resp := BridgestrapResponse{}
		// Note that this request can take up to one minute to complete because
		// bridgestrap's timeout is 60 seconds.
		m.MakeRequest(req, &resp)

		if resp.Functional {
			r.SetState(core.StateFunctional)
		} else {
			log.Printf("%q not functional because %q", r.String(), resp.Error)
			r.SetState(core.StateNotFunctional)
		}
	}
}

// reloadBridgeDescriptors reloads bridge descriptor from the given file.
func reloadBridgeDescriptors(extrainfoFile string, bag ResourceCollection) {

	var err error
	var res []core.Resource
	log.Println("Reloading bridge descriptors.")

	res, err = loadBridgesFromExtrainfo(extrainfoFile)
	if err != nil {
		log.Printf("Failed to reload bridge descriptors: %s", err)
	} else {
		log.Printf("Successfully reloaded %d bridge descriptors.", len(res))
	}

	log.Println("Adding new resources.")
	for _, resource := range res {
		bag.Add(resource.Name(), resource)
	}
	log.Println("Done adding new resources.")
}

// loadBridgesFromExtrainfo loads and returns bridges from Serge's extrainfo
// files.
func loadBridgesFromExtrainfo(extrainfoFile string) ([]core.Resource, error) {

	file, err := os.Open(extrainfoFile)
	if err != nil {
		log.Printf("Failed to open extrainfo file: %s", err)
		return nil, err
	}
	defer file.Close()

	extra, err := ParseExtrainfoDoc(file)
	if err != nil {
		log.Printf("Failed to read bridges from extrainfo file: %s", err)
		return nil, err
	}

	return extra, nil
}

// ParseExtrainfoDoc parses the given extra-info document and returns the
// content as a Bridges object.  Note that the extra-info document format is as
// it's produced by the bridge authority.
func ParseExtrainfoDoc(r io.Reader) ([]core.Resource, error) {

	var fingerprint string
	var transports []core.Resource
	// var bridges = rsrc.NewBridges()
	// var b *rsrc.Bridge

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		// We're dealing with a new extra-info block, i.e., a new bridge.
		if strings.HasPrefix(line, ExtraInfoPrefix) {
			// b = rsrc.NewBridge()
			words := strings.Split(line, " ")
			if len(words) != 3 {
				return nil, errors.New("incorrect number of words in 'extra-info' line")
			}
			fingerprint = words[2]
			// bridges.Bridges[b.Fingerprint] = b
		}
		// We're dealing with a bridge's transport protocols.  There may be
		// several.
		if strings.HasPrefix(line, TransportPrefix) {
			t := resources.NewTransport()
			t.Fingerprint = fingerprint
			err := populateTransportInfo(line, t)
			if err != nil {
				return nil, err
			}
			// b.AddTransport(t)
			transports = append(transports, t)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return transports, nil
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
	t.Type = words[1]

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
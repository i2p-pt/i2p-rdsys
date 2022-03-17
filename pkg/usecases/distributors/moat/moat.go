package moat

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log"
	mrand "math/rand"
	"sync"
	"time"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/delivery"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/delivery/mechanisms"
)

const (
	DistName              = "moat"
	builtinRefreshSeconds = time.Hour
)

var (
	NoTransportError = errors.New("No provided transport is available for this country")
)

// CircumventionMap maps countries to the CircumventionSettings that ara available on those countries
type CircumventionMap map[string]CircumventionSettings

type CircumventionSettings struct {
	Settings []Settings `json:"settings"`
}

type Settings struct {
	Bridges BridgeSettings `json:"bridges"`
}

type BridgeSettings struct {
	Type          string   `json:"type"`
	Source        string   `json:"source"`
	BridgeStrings []string `json:"bridge_strings,omitempty"`
}

type MoatDistributor struct {
	collection            core.Collection
	builtinBridges        map[string][]string
	circumventionMap      CircumventionMap
	circumventionDefaults CircumventionSettings
	cfg                   *internal.MoatDistConfig
	ipc                   delivery.Mechanism
	wg                    sync.WaitGroup
	shutdown              chan bool

	FetchBridges func(url string) (bridgeLines []string, err error)
}

func (d *MoatDistributor) LoadCircumventionMap(r io.Reader) error {
	dec := json.NewDecoder(r)
	return dec.Decode(&d.circumventionMap)
}

func (d *MoatDistributor) LoadCircumventionDefaults(r io.Reader) error {
	dec := json.NewDecoder(r)
	return dec.Decode(&d.circumventionDefaults)
}

func (d *MoatDistributor) GetCircumventionMap() CircumventionMap {
	return d.circumventionMap
}

func (d *MoatDistributor) GetCircumventionSettings(country string, types []string) (*CircumventionSettings, error) {
	cc, ok := d.circumventionMap[country]
	if !ok || len(cc.Settings) == 0 {
		return nil, nil
	}
	return d.populateCircumventionSettings(&cc, types)
}

func (d *MoatDistributor) GetCircumventionDefaults(types []string) (*CircumventionSettings, error) {
	return d.populateCircumventionSettings(&d.circumventionDefaults, types)
}

func (d *MoatDistributor) populateCircumventionSettings(cc *CircumventionSettings, types []string) (*CircumventionSettings, error) {
	circumventionSettings := CircumventionSettings{make([]Settings, 0, len(cc.Settings))}
	for _, settings := range cc.Settings {
		if len(types) != 0 {
			requestedType := false
			for _, t := range types {
				if t == settings.Bridges.Type {
					requestedType = true
					break
				}
			}

			if !requestedType {
				continue
			}
		}

		settings.Bridges.BridgeStrings = d.getBridges(settings.Bridges)
		circumventionSettings.Settings = append(circumventionSettings.Settings, settings)
	}

	if len(circumventionSettings.Settings) == 0 {
		log.Println("Could not find the requested type of bridge", types)
		return nil, NoTransportError
	}

	return &circumventionSettings, nil
}

func (d *MoatDistributor) getBridges(bs BridgeSettings) []string {
	log.Println("type:", bs.Type)
	var bridgestrings []string
	switch bs.Source {
	case "builtin":
		bridgeList := d.builtinBridges[bs.Type]
		if len(bridgeList) <= d.cfg.NumBridgesPerRequest {
			bridgestrings = bridgeList
		} else {
			for i := 0; i < d.cfg.NumBridgesPerRequest; i++ {
				index := mrand.Intn(len(bridgeList))
				bridgestrings = append(bridgestrings, bridgeList[index])
			}
		}
	case "bridgedb":
		for i := 0; i < d.cfg.NumBridgesPerRequest; i++ {
			id := make([]byte, 8)
			rand.Read(id)
			bridge, err := d.collection[bs.Type].Get(core.NewHashkey(string(id)))
			if err != nil {
				log.Println("Can't get bridgedb bridges of type", bs.Type, ":", err)
			} else {
				bridgestrings = append(bridgestrings, bridge.String())
			}
		}
	default:
		log.Println("Requested an unsuported bridge source:", bs.Source)
	}
	return bridgestrings
}

func (d *MoatDistributor) GetBuiltInBridges(types []string) map[string][]string {
	builtinBridges := map[string][]string{}
	if len(types) == 0 {
		builtinBridges = d.builtinBridges
	}

	for _, t := range types {
		bridges, ok := d.builtinBridges[t]
		if ok {
			builtinBridges[t] = bridges
		}
	}

	for _, bridges := range builtinBridges {
		mrand.Shuffle(len(bridges), func(i, j int) { bridges[i], bridges[j] = bridges[j], bridges[i] })
	}
	return builtinBridges
}

// housekeeping listens to updates from the backend resources
func (d *MoatDistributor) housekeeping(rStream chan *core.ResourceDiff) {
	defer d.wg.Done()
	defer close(rStream)
	defer d.ipc.StopStream()

	ticker := time.NewTimer(builtinRefreshSeconds)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.fetchBuiltinBridges()
		case diff := <-rStream:
			d.collection.ApplyDiff(diff)
		case <-d.shutdown:
			log.Printf("Shutting down housekeeping.")
			return
		}
	}
}

func (d *MoatDistributor) fetchBuiltinBridges() {
	for _, bType := range d.cfg.BuiltInBridgesTypes {
		builtinBridges, err := d.FetchBridges(d.cfg.BuiltInBridgesURL + "bridges_list." + bType + ".txt")
		if err != nil {
			log.Println("Failed to fetch builtin bridges of type", bType, ":", err)
			continue
		}
		d.builtinBridges[bType] = builtinBridges
	}
}

func (d *MoatDistributor) Init(cfg *internal.Config) {
	log.Printf("Initialising %s distributor.", DistName)
	mrand.Seed(time.Now().UnixNano())

	d.cfg = &cfg.Distributors.Moat
	d.shutdown = make(chan bool)
	d.collection = core.NewCollection()
	for _, rType := range d.cfg.Resources {
		d.collection.AddResourceType(rType, true, nil)
	}

	d.builtinBridges = make(map[string][]string)
	d.fetchBuiltinBridges()

	log.Printf("Initialising resource stream.")
	d.ipc = mechanisms.NewHttpsIpc(
		"http://"+cfg.Backend.WebApi.ApiAddress+cfg.Backend.ResourceStreamEndpoint,
		"GET",
		cfg.Backend.ApiTokens[DistName])
	rStream := make(chan *core.ResourceDiff)
	req := core.ResourceRequest{
		RequestOrigin: "settings",
		ResourceTypes: d.cfg.Resources,
		Receiver:      rStream,
	}
	d.ipc.StartStream(&req)

	d.wg.Add(1)
	go d.housekeeping(rStream)
}

func (d *MoatDistributor) Shutdown() {
	log.Printf("Shutting down %s distributor.", DistName)

	close(d.shutdown)
	d.wg.Wait()
}

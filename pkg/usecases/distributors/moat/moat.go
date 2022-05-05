// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package moat

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	mrand "math/rand"
	"net"
	"strconv"
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
	Country  string     `json:"country,omitempty"`
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

func (d *MoatDistributor) GetCircumventionSettings(country string, types []string, ip net.IP) (*CircumventionSettings, error) {
	cc, ok := d.circumventionMap[country]
	cc.Country = country
	if !ok || len(cc.Settings) == 0 {
		// json.Marshal will return null for an empty slice unless we *make* it
		cc.Settings = make([]Settings, 0)
		return &cc, nil
	}
	return d.populateCircumventionSettings(&cc, types, ip)
}

func (d *MoatDistributor) GetCircumventionDefaults(types []string, ip net.IP) (*CircumventionSettings, error) {
	return d.populateCircumventionSettings(&d.circumventionDefaults, types, ip)
}

func (d *MoatDistributor) populateCircumventionSettings(cc *CircumventionSettings, types []string, ip net.IP) (*CircumventionSettings, error) {
	circumventionSettings := CircumventionSettings{
		Settings: make([]Settings, 0, len(cc.Settings)),
		Country:  cc.Country,
	}

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

		settings.Bridges.BridgeStrings = d.getBridges(settings.Bridges, ip)
		circumventionSettings.Settings = append(circumventionSettings.Settings, settings)
	}

	if len(circumventionSettings.Settings) == 0 {
		log.Println("Could not find the requested type of bridge", types)
		return nil, NoTransportError
	}

	return &circumventionSettings, nil
}

func (d *MoatDistributor) getBridges(bs BridgeSettings, ip net.IP) []string {
	switch bs.Source {
	case "builtin":
		bridges := d.GetBuiltInBridges([]string{bs.Type})
		return bridges[bs.Type]

	case "bridgedb":
		hashring := d.collection.GetHashring(d.getProportionIndex(), bs.Type)
		var resources []core.Resource
		if hashring.Len() <= d.cfg.NumBridgesPerRequest {
			resources = hashring.GetAll()
		} else {
			var err error
			resources, err = hashring.GetMany(ipHashkey(ip), d.cfg.NumBridgesPerRequest)
			if err != nil {
				log.Println("Error getting resources from the subhashring:", err)
			}
		}
		bridgestrings := []string{}
		for _, resource := range resources {
			bridgestrings = append(bridgestrings, resource.String())
		}
		return bridgestrings

	default:
		log.Println("Requested an unsuported bridge source:", bs.Source)
		return []string{}
	}
}

func ipHashkey(ip net.IP) core.Hashkey {
	mask := net.CIDRMask(32, 128)
	if ip.To4() != nil {
		mask = net.CIDRMask(16, 32)
	}
	return core.NewHashkey(ip.Mask(mask).String())
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
	proportions := d.makeProportions()
	for _, rType := range d.cfg.Resources {
		d.collection.AddResourceType(rType, len(proportions) == 0, proportions)
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

func (d *MoatDistributor) makeProportions() map[string]int {
	proportions := make(map[string]int)
	for i := 0; i < d.cfg.NumPeriods; i++ {
		proportions[strconv.Itoa(i)] = 1
	}
	return proportions
}

func (d *MoatDistributor) getProportionIndex() string {
	if d.cfg.NumPeriods == 0 || d.cfg.RotationPeriodHours == 0 {
		return ""
	}

	now := int(time.Now().Unix() / (60 * 60))
	period := now / d.cfg.RotationPeriodHours
	return strconv.Itoa(period % d.cfg.NumPeriods)
}

func (d *MoatDistributor) Shutdown() {
	log.Printf("Shutting down %s distributor.", DistName)

	close(d.shutdown)
	d.wg.Wait()
}

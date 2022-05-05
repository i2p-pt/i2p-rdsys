// Copyright (c) 2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

type bridgesJSON struct {
	Bridgelines []string `json:"bridgelines"`
}

func (d *TelegramDistributor) loadNewBridgesFromStore() {
	d.newHashrightLock.Lock()
	defer d.newHashrightLock.Unlock()

	for updater, store := range d.NewBridgesStore {
		var rs []resources.Transport
		err := store.Load(&rs)
		if err != nil {
			log.Println("Error loading updater", updater, ":", err)
			continue
		}
		for _, r := range rs {
			d.newHashring.Add(&r)
		}
	}
}

// LoadNewBridges loads bridges in bridgesJSON format from the reader into the new bridges newHashring
//
// This function locks a mutex when accessing the newHashring, we should be careful to don't make
// a deadlock with the internal mutex in the hashring. Never call this function while holding the
// newHashring mutex.
func (d *TelegramDistributor) LoadNewBridges(name string, r io.Reader) error {
	var updatedBridges bridgesJSON
	dec := json.NewDecoder(r)
	err := dec.Decode(&updatedBridges)
	if err != nil {
		return err
	}

	resources := make([]core.Resource, len(updatedBridges.Bridgelines))
	for i, bridgeline := range updatedBridges.Bridgelines {
		resource, err := parseBridgeline(bridgeline)
		if err != nil {
			return err
		}
		if resource.Type() != d.cfg.Resource {
			return fmt.Errorf("Not valid bridge type %s", resource.Type())
		}

		resources[i] = resource
	}

	d.newHashrightLock.Lock()
	for _, resource := range d.dynamicBridges[name] {
		d.newHashring.Remove(resource)
	}
	d.dynamicBridges[name] = resources

	for _, resource := range resources {
		d.newHashring.Add(resource)
	}
	d.newHashrightLock.Unlock()

	log.Println("Got", len(resources), "new bridges from", name)

	persistence := d.NewBridgesStore[name]
	if persistence != nil {
		return d.NewBridgesStore[name].Save(resources)
	}

	return nil
}

func parseBridgeline(bridgeline string) (core.Resource, error) {
	bridgeParts := strings.Split(bridgeline, " ")

	var bridge resources.Transport
	bridge.RType = bridgeParts[1]
	bridge.Fingerprint = bridgeParts[3]

	addrParts := strings.Split(bridgeParts[2], ":")
	if len(addrParts) != 2 {
		return nil, fmt.Errorf("Malformed address %s", bridgeParts[2])
	}
	addr, err := net.ResolveIPAddr("", addrParts[0])
	if err != nil {
		return nil, err
	}
	bridge.Address = resources.IPAddr{IPAddr: *addr}
	port, err := strconv.Atoi(addrParts[1])
	if err != nil {
		return nil, fmt.Errorf("Can't convert port to integer: %s", err)
	}
	bridge.Port = uint16(port)

	bridge.Parameters = make(map[string]string)
	for _, param := range bridgeParts[4:] {
		paramParts := strings.Split(param, "=")
		if len(paramParts) != 2 {
			return nil, fmt.Errorf("Malformed param %s", param)
		}
		bridge.Parameters[paramParts[0]] = paramParts[1]
	}
	return &bridge, nil
}

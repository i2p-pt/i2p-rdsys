// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"encoding/json"
	"log"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/persistence"
	pjson "gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/persistence/json"
)

// ResourceStore keeps resources in persistant storage
type ResourceStore struct {
	rMech map[string]persistence.Mechanism
	rcol  *core.BackendResources
}

// InitResourceStore loading into rcol all the existing resources in the persitant storage
func InitResourceStore(cfg *Config, rcol *core.BackendResources) *ResourceStore {
	store := ResourceStore{
		rMech: make(map[string]persistence.Mechanism),
		rcol:  rcol,
	}

	for rType, conf := range cfg.Backend.Resources {
		if !conf.Stored {
			continue
		}

		store.rMech[rType] = pjson.New(rType, cfg.Backend.StorageDir)

		var rawResources []json.RawMessage
		err := store.rMech[rType].Load(&rawResources)
		if err != nil {
			log.Printf("Can't load resources of type %s: %v", rType, err)
			continue
		}

		resources, err := UnmarshalResources(rawResources)
		if err != nil {
			log.Printf("Error unmarshalling %s storage resources: %s", rType, err)
			continue
		}

		for _, r := range resources {
			err = rcol.Collection[rType].Add(r)
			if err != nil {
				log.Println("Error adding resource to the collection:", err)
			}
		}
	}

	return &store
}

// Save the resources of type rType into it's persistant storage
func (store *ResourceStore) Save(rType string) {
	rMech, ok := store.rMech[rType]
	if !ok {
		return
	}

	resources := store.rcol.Collection[rType].GetAll()
	err := rMech.Save(resources)
	if err != nil {
		log.Println("Error saving resource on storage:", err)
	}
}

// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"fmt"
	"strings"
	"testing"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

var (
	config = internal.Config{
		Distributors: internal.Distributors{
			Telegram: internal.TelegramDistConfig{
				Resource:             "dummy",
				NumBridgesPerRequest: 1,
				RotationPeriodHours:  1,
				MinUserID:            100,
			},
		},
	}

	oldDummyResource = core.NewDummy(core.NewHashkey("old-oid"), core.NewHashkey("old-uid"))
	newDummyResource = core.NewDummy(core.NewHashkey("new-oid"), core.NewHashkey("new-uid"))
)

func initDistributor() *TelegramDistributor {
	d := TelegramDistributor{}
	d.Init(&config)
	d.newHashring.Add(newDummyResource)
	d.oldHashring.Add(oldDummyResource)
	return &d
}

func TestGetResources(t *testing.T) {
	newID := int64(101)
	oldID := int64(10)

	d := initDistributor()
	defer d.Shutdown()

	res := d.GetResources(newID)
	if len(res) != 1 {
		t.Fatalf("Wrong number of resrources for new: %d", len(res))
	}
	if res[0] != newDummyResource {
		t.Errorf("Wrong resource: %v", res[0])
	}

	res = d.GetResources(oldID)
	if len(res) != 2 {
		t.Fatalf("Wrong number of resrources for old: %d", len(res))
	}
	if res[0] != oldDummyResource {
		t.Errorf("Wrong resource: %v", res[0])
	}
	if res[1] != newDummyResource {
		t.Errorf("Wrong resource: %v", res[1])
	}
}

func TestLoadNewResources(t *testing.T) {
	tpe := "obfs4"
	ip := "100.77.53.79"
	port := uint16(38248)
	fingerprint := "7DFCB47E84DA8F6D1030F370F2E308D574281E77"
	params := map[string]string{
		"public-key": "61126de1b795b976f3ac878f48e88fa77a87d7308ba57c7642b9e1068403a496",
		"iat-mode":   "0",
	}

	d := TelegramDistributor{}
	c := config
	c.Distributors.Telegram.Resource = tpe
	d.Init(&c)
	defer d.Shutdown()

	r := strings.NewReader(fmt.Sprintf("%s %s:%d %s public-key=%s iat-mode=%s", tpe, ip, port, fingerprint, params["public-key"], params["iat-mode"]))
	err := d.LoadNewBridges(r)
	if err != nil {
		t.Fatalf("Error loading new bridges: %v", err)
	}
	rs := d.newHashring.GetAll()
	if len(rs) != 1 {
		t.Fatalf("Wrong number of resources: %d", len(rs))
	}
	bridge, ok := rs[0].(*resources.Transport)
	if !ok {
		t.Fatalf("Resource is not a transport: %s", rs[0].String())
	}

	if bridge.Type() != tpe {
		t.Errorf("Wrong type: %s", bridge.Type())
	}
	if bridge.Address.String() != ip {
		t.Errorf("Wrong ip: %s", bridge.Address.String())
	}
	if bridge.Port != port {
		t.Errorf("Wrong port: %d", bridge.Port)
	}
	if bridge.Fingerprint != fingerprint {
		t.Errorf("Wrong fingerprint: %s", bridge.Fingerprint)
	}
	if len(bridge.Parameters) != 2 {
		t.Errorf("Wrong parameters: %v", bridge.Parameters)
	}
	for k, v := range params {
		if bridge.Parameters[k] != v {
			t.Errorf("Wrong parameter %s: %s", k, bridge.Parameters[k])
		}
	}
}

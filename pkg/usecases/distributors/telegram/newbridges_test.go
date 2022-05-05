// Copyright (c) 2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"fmt"
	"strings"
	"testing"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

const (
	tpe          = "obfs4"
	ip           = "100.77.53.79"
	port         = uint16(38248)
	fingerprint  = "7DFCB47E84DA8F6D1030F370F2E308D574281E77"
	fingerprint2 = "AAAAB47E84DA8F6D1030F370F2E308D574281E77"
)

var (
	params = map[string]string{
		"cert":     "61126de1b795b976f3ac878f48e88fa77a87d7308ba57c7642b9e1068403a496",
		"iat-mode": "0",
	}
)

func TestLoadNewResources(t *testing.T) {
	d := TelegramDistributor{}
	c := config
	c.Distributors.Telegram.Resource = tpe
	d.Init(&c)
	defer d.Shutdown()

	r := strings.NewReader(fmt.Sprintf(`{
		"bridgelines": [
			"Bridge %s %s:%d %s cert=%s iat-mode=%s"
		]
		}`, tpe, ip, port, fingerprint, params["cert"], params["iat-mode"]))
	err := d.LoadNewBridges("updater", r)
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

func TestUpdateNewResources(t *testing.T) {
	d := TelegramDistributor{}
	c := config
	c.Distributors.Telegram.Resource = tpe
	d.Init(&c)
	defer d.Shutdown()

	r := strings.NewReader(fmt.Sprintf(`{
		"bridgelines": [
			"Bridge %s %s:%d %s cert=%s iat-mode=%s"
		]
		}`, tpe, ip, port, fingerprint, params["cert"], params["iat-mode"]))
	err := d.LoadNewBridges("updater", r)
	if err != nil {
		t.Fatalf("Error loading new bridges: %v", err)
	}

	r = strings.NewReader(fmt.Sprintf(`{
		"bridgelines": [
			"Bridge %s %s:%d %s cert=%s iat-mode=%s"
		]
		}`, tpe, ip, port, fingerprint2, params["cert"], params["iat-mode"]))
	err = d.LoadNewBridges("updater", r)
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
	if bridge.Fingerprint != fingerprint2 {
		t.Errorf("Wrong fingerprint: %s", bridge.Fingerprint)
	}
}

func TestLoadNewResourcesMultipleUpdaters(t *testing.T) {
	d := TelegramDistributor{}
	c := config
	c.Distributors.Telegram.Resource = tpe
	d.Init(&c)
	defer d.Shutdown()

	r := strings.NewReader(fmt.Sprintf(`{
		"bridgelines": [
			"Bridge %s %s:%d %s cert=%s iat-mode=%s"
		]
		}`, tpe, ip, port, fingerprint, params["cert"], params["iat-mode"]))
	err := d.LoadNewBridges("updater", r)
	if err != nil {
		t.Fatalf("Error loading new bridges: %v", err)
	}

	r = strings.NewReader(fmt.Sprintf(`{
		"bridgelines": [
			"Bridge %s %s:%d %s cert=%s iat-mode=%s"
		]
		}`, tpe, ip, port, fingerprint2, params["cert"], params["iat-mode"]))
	err = d.LoadNewBridges("updater2", r)
	if err != nil {
		t.Fatalf("Error loading new bridges: %v", err)
	}
	rs := d.newHashring.GetAll()
	if len(rs) != 2 {
		t.Fatalf("Wrong number of resources: %d", len(rs))
	}
}

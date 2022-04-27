// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"testing"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

var (
	testCfg = Config{
		Backend: BackendConfig{
			ExtrainfoFile:     "./test_assets/cached-extrainfo",
			NetworkstatusFile: "./test_assets/networkstatus-bridges",
			DescriptorsFile:   "./test_assets/bridge-descriptors",
			DistProportions:   map[string]int{"moat": 1, "https": 0, "email": 0},
		},
	}
	resourceTypes = []string{"vanilla", "obfs4"}
	distributor   = map[string][]string{
		"moat":  {"0469A5A09C3DA2E56E9EE1D251EAD5D12FA6ECEE", "AA6CFB09DD3C5468C8572E0E78A9717EE3894737", "12DE44C452F03701E4B9E722A056CE53258F8FE8"},
		"https": {"1F8A76D9581D72B9B9D84411463445052A78AB71", "97742B46FFFDAD3E703BA564B3D920739FDA4F38", "49FD7C6D391FB40EC498B04DB4B75599D0D77BE0"},
		"email": {"56E04AE5C0F64F22206A49939B33FB597BFE1AA7", "439B8DF324C99FBEBE49344D61C93244C773E402", "7054D84C8C8127CF914E1949ECFC5DAA5746B4C6"},
		"none":  {"7C213E44DF0C74777033B33E3366A8967100B8A5", "B20383C0D841CC31BCECD79C46B786CDE8E807AE", "155F8662F72A330FFBFB373296D44623608FD0AB"},
		"any":   {"768825A19A46DA68FD72FE9222C66A4E7ADE9CD1", "636314F19ED47A448AA6B54E491EEB822523588F", "C518EC4F6B42AB6EA1F45274B85F4DD72E1E1DD1"},
	}
)

func TestDistributionMechanism(t *testing.T) {
	rcol := core.NewBackendResources()
	for _, rType := range resourceTypes {
		rcol.AddResourceType(rType, false, testCfg.Backend.DistProportions)
	}
	reloadBridgeDescriptors(&testCfg, rcol, nil)

	foundAny := make([]bool, len(distributor["any"]))
	for distName := range testCfg.Backend.DistProportions {
		rs := rcol.Get(distName, "obfs4")
		found := make([]bool, len(distributor[distName]))
		for _, res := range rs {
			transport, ok := res.(*resources.Transport)
			if !ok {
				continue
			}

			for d, fps := range distributor {
				for i, fp := range fps {
					if transport.Fingerprint == fp {
						if d == distName {
							found[i] = true
						} else if distName == "moat" && d == "any" {
							foundAny[i] = true
						} else {
							t.Errorf("%s found in %s but should be in %s", fp, distName, d)
						}
						break
					}
				}
			}
		}

		for i, f := range found {
			if !f {
				t.Errorf("%s not found in %s", distributor[distName][i], distName)
			}
		}
	}
	for i, f := range foundAny {
		if !f {
			t.Errorf("%s with 'any' distribution request not found in moat", distributor["any"][i])
		}
	}
}

func TestDistributionMechanismUpdated(t *testing.T) {
	fp := "56E04AE5C0F64F22206A49939B33FB597BFE1AA7"

	rcol := core.NewBackendResources()
	for _, rType := range resourceTypes {
		rcol.AddResourceType(rType, false, testCfg.Backend.DistProportions)
	}

	reloadBridgeDescriptors(&testCfg, rcol, nil)
	rs := rcol.Get("email", "obfs4")
	found := false
	for _, res := range rs {
		transport, ok := res.(*resources.Transport)
		if ok && transport.Fingerprint == fp {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("Not found %s in email", fp)
	}

	cfg := testCfg
	cfg.Backend.DescriptorsFile = "./test_assets/bridge-descriptors_update"
	reloadBridgeDescriptors(&cfg, rcol, nil)
	rs = rcol.Get("moat", "obfs4")
	found = false
	for _, res := range rs {
		transport, ok := res.(*resources.Transport)
		if ok && transport.Fingerprint == fp {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Not found %s after being changed to moat", fp)
	}
}

func TestOnlyFunctional(t *testing.T) {
	fpDysfucntional := "56E04AE5C0F64F22206A49939B33FB597BFE1AA7"
	fpFunctional := "439B8DF324C99FBEBE49344D61C93244C773E402"

	rcol := core.NewBackendResources()
	for _, rType := range resourceTypes {
		rcol.AddResourceType(rType, false, testCfg.Backend.DistProportions)
	}
	metrics := InitMetrics()

	reloadBridgeDescriptors(&testCfg, rcol, nil)
	calcTestedResources(metrics, rcol)
	if rcol.OnlyFunctional {
		t.Errorf("OnlyFunctional flag enabled when most resources are untested")
	}

	rs := rcol.Get("email", "obfs4")
	foundFunc := false
	foundDys := false
	for _, res := range rs {
		transport, ok := res.(*resources.Transport)
		if ok {
			if transport.Fingerprint == fpDysfucntional {
				foundDys = true
			} else if transport.Fingerprint == fpFunctional {
				foundFunc = true
			}
		}
	}
	if !foundFunc {
		t.Errorf("Not found %s in email", fpFunctional)
	}
	if !foundDys {
		t.Errorf("Not found %s in email", fpFunctional)
	}

	for _, hashring := range rcol.Collection {
		for _, r := range hashring.GetAll() {
			transport, ok := r.(*resources.Transport)
			if ok && transport.Fingerprint == fpDysfucntional {
				r.TestResult().State = core.StateDysfunctional
				continue
			}
			r.TestResult().State = core.StateFunctional
		}
	}
	calcTestedResources(metrics, rcol)
	if !rcol.OnlyFunctional {
		t.Errorf("OnlyFunctional flag disabled when most resources are functional")
	}

	rs = rcol.Get("email", "obfs4")
	if len(rs) == 0 {
		t.Fatalf("No resources in email distributor")
	}

	foundFunc = false
	foundDys = false
	for _, res := range rs {
		transport, ok := res.(*resources.Transport)
		if ok {
			if transport.Fingerprint == fpDysfucntional {
				foundDys = true
			} else if transport.Fingerprint == fpFunctional {
				foundFunc = true
			}
		}
	}
	if !foundFunc {
		t.Errorf("Not found functional bridge %s in email", fpFunctional)
	}
	if foundDys {
		t.Errorf("Found dysfunctional bridge %s in email", fpDysfucntional)
	}
}

// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"testing"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
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

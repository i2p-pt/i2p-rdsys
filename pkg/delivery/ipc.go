// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package delivery

import (
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
)

type Mechanism interface {
	StartStream(*core.ResourceRequest)
	StopStream()
	MakeJsonRequest(interface{}, interface{}) error
}

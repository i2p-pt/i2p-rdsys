// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package distributors

import (
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
)

// Distributor represents a distribution mechanism, e.g. Salmon or HTTPS.
type Distributor interface {
	Init(*internal.Config)
	Shutdown()
}

// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package persistence

// Mechanism represents a persistence mechanism, i.e. a mechanism that can
// store data on a persistent medium.  This could be a flat file, a SQL
// database, or a blockchain.
type Mechanism interface {
	Load(interface{}) error
	Save(interface{}) error
}

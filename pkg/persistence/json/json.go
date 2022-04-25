// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package json

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
)

const (
	PersistenceMethod = "json"
)

type JsonPersistence struct {
	filename string
}

// Load decodes the content of f.filename and writes the result to the given
// interface.
func (f *JsonPersistence) Load(i interface{}) error {
	log.Printf("Attempting to load state from %q.", f.filename)

	fh, err := os.Open(f.filename)
	if err != nil {
		return err
	}
	defer fh.Close()

	dec := json.NewDecoder(fh)
	return dec.Decode(i)
}

// Save encodes the given interface to f.filename.
func (f *JsonPersistence) Save(i interface{}) error {
	log.Printf("Attempting to save state to %q.", f.filename)

	dirPath := path.Dir(f.filename)
	os.MkdirAll(dirPath, 0700)

	fh, err := os.Create(f.filename)
	if err != nil {
		return err
	}
	defer fh.Close()

	enc := json.NewEncoder(fh)
	return enc.Encode(i)
}

// New returns a new JsonPersistence instance.
func New(name string, workingDir string) *JsonPersistence {
	file := fmt.Sprintf("%s.json", name)
	filename := path.Join(workingDir, file)
	return &JsonPersistence{filename: filename}
}

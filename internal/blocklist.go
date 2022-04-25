// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"bufio"
	"log"
	"os"
	"regexp"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
)

var (
	fpCountryExp = regexp.MustCompile(`^fingerprint ([0-9A-F]+) country-code ([a-z]+)$`)
)

type blocklist struct {
	// map[fingerprint]map[country]
	blocked map[string]core.LocationSet

	// map[country][]fingperprints
	allowed map[string]map[string]struct{}
}

func newBlockList(blockFile, allowFile string) (*blocklist, error) {
	bl := blocklist{
		blocked: make(map[string]core.LocationSet),
		allowed: make(map[string]map[string]struct{}),
	}

	blocklist, err := parseBlockAllowList(blockFile)
	if err != nil {
		return nil, err
	}
	for _, pair := range blocklist {
		fp := pair[0]
		country := pair[1]
		if _, ok := bl.blocked[fp]; !ok {
			bl.blocked[fp] = core.LocationSet{}
		}
		bl.blocked[fp][country] = true
	}

	allowlist, err := parseBlockAllowList(allowFile)
	if err != nil {
		return nil, err
	}
	for _, pair := range allowlist {
		fp := pair[0]
		country := pair[1]
		if _, ok := bl.allowed[country]; !ok {
			bl.allowed[country] = make(map[string]struct{})
		}
		bl.allowed[country][fp] = struct{}{}
	}

	return &bl, nil
}

func parseBlockAllowList(listFile string) ([][2]string, error) {
	list := [][2]string{}
	if listFile == "" {
		return list, nil
	}

	file, err := os.Open(listFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		re := fpCountryExp.FindStringSubmatch(line)
		if len(re) != 3 {
			log.Printf("Wrong blocklist format (%s): %s", listFile, line)
			continue
		}
		list = append(list, [2]string{re[1], re[2]})
	}

	err = scanner.Err()
	return list, err
}

func (bl *blocklist) blockedIn(fingerprint string) core.LocationSet {
	blockCountries := bl.blocked[fingerprint]
	if blockCountries == nil {
		blockCountries = core.LocationSet{}
	}

	for country, fps := range bl.allowed {
		if _, ok := fps[fingerprint]; ok {
			delete(blockCountries, country)
		} else {
			blockCountries[country] = true
		}
	}

	return blockCountries
}

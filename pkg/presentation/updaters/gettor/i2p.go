// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gettor

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"path"

	//"github.com/google/go-github/github"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

const (
	i2pPlatform = "github"
)

type i2pProvider struct {
	ctx   context.Context
	cfg   *internal.I2P
	cache map[string]*resources.TBLink
}

func newI2PProvider(cfg *internal.I2P) *i2pProvider {
	ctx := context.Background()
	return &i2pProvider{ctx, cfg, make(map[string]*resources.TBLink)}
}

//needsUpdate(platform string, version resources.Version) bool
func (i *i2pProvider) needsUpdate(platform string, version resources.Version) bool {
	release, err := i.getRelease()
	if err != nil {
		log.Println("[I2P] Error fetching latest release:", err)
		return false
	}
	cached, ok := i.cache[platform]
	if !ok {
		log.Println("[I2P] No cached release for", platform)
		return true
	}
	cachedVersion, err := resources.Str2Version(cached.Version.String())
	if err != nil {
		log.Println("[I2P] Error parsing cached version:", err)
		return true
	}
	if version.Compare(cachedVersion) == 1 {
		log.Println("[I2P] New version available:", version, ">", cachedVersion)
		return true
	}
	releaseVersion, err := resources.Str2Version(release)
	if err != nil {
		log.Println("[I2P] Error parsing latest release:", err)
		return true
	}
	if version.Compare(releaseVersion) == 1 {
		log.Println("[I2P] New version available:", version, ">", releaseVersion)
		return true
	}
	return false
}

//newRelease(platform string, version resources.Version) uploadFileFunc

func (i *i2pProvider) newRelease(platform string, version resources.Version) uploadFileFunc {
	return func(binaryPath string, sigPath string, locale string) *resources.TBLink {
		if _, ok := i.cache[platform]; !ok {
			i.cache[platform] = resources.NewTBLink()
		}
		for index, filePath := range []string{binaryPath, sigPath} {
			filename := path.Base(filePath)
			if index == 0 {
				MagnetLink, err := i.cache[platform].GenerateFileMagnet(filename)
				if err != nil {
					log.Println("[I2P] Couldn't generate a magnet link for", filename, ":", err)
					return nil
				}
				i.cache[platform].Link = MagnetLink
			} else {
				SigMagnetLink, err := i.cache[platform].GenerateSigMagnet(filename)
				if err != nil {
					log.Println("[I2P] Couldn't generate a magnet link for", filename, ":", err)
					return nil
				}
				i.cache[platform].SigLink = SigMagnetLink
			}
		}

		i.cache[platform].Version = version
		i.cache[platform].Provider = i2pPlatform
		i.cache[platform].Platform = platform
		i.cache[platform].Locale = locale
		i.cache[platform].FileName = path.Base(binaryPath)

		return i.cache[platform]
	}
}

func (i *i2pProvider) getRelease() (string, error) {
	// get the latest version for the platform from "https://aus1.torproject.org/torbrowser/update_3/release/downloads.json"
	// return the list of releases for the platform
	// return an error if the platform is not supported
	// start by fetching the downloads.json
	resp, err := http.Get("https://aus1.torproject.org/torbrowser/update_3/release/downloads.json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	// read the body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	// parse the body
	var downloads map[string]interface{}
	err = json.Unmarshal(body, &downloads)
	if err != nil {
		return "", err
	}
	v := downloads["version"].(string)
	log.Printf("[I2P] latest Tor version fetched: %s", v)

	return v, nil
}

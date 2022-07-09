// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gettor

import (
	"context"
	"log"
	"path"

	//"github.com/google/go-github/github"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

const (
	i2pPlatform = "github"
)

type i2pProvider struct {
	ctx context.Context
	cfg *internal.I2P
}

func newI2PProvider(cfg *internal.I2P) *i2pProvider {
	ctx := context.Background()
	return &i2pProvider{ctx, cfg}
}

func (gh *i2pProvider) newRelease(platform string, version resources.Version) uploadFileFunc {
	return func(binaryPath string, sigPath string, locale string) *resources.TBLink {
		link := resources.NewTBLink()
		for i, filePath := range []string{binaryPath, sigPath} {
			filename := path.Base(filePath)
			if i == 0 {
				MagnetLink, err := link.GenerateFileMagnet(filename)
				if err != nil {
					log.Println("[I2P] Couldn't generate a magnet link for", filename, ":", err)
					return nil
				}
				link.Link = MagnetLink
			} else {
				SigMagnetLink, err := link.GenerateSigMagnet(filename)
				if err != nil {
					log.Println("[I2P] Couldn't generate a magnet link for", filename, ":", err)
					return nil
				}
				link.SigLink = SigMagnetLink
			}
		}

		link.Version = version
		link.Provider = i2pPlatform
		link.Platform = platform
		link.Locale = locale
		link.FileName = path.Base(binaryPath)

		return link
	}
}

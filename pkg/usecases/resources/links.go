// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resources

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xgfone/bt/bencode"
	"github.com/xgfone/bt/metainfo"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
)

type Version struct {
	Mayor int `json:"mayor"`
	Minor int `json:"minor"`
	Patch int `json:"patch"`
}

func Str2Version(s string) (version Version, err error) {
	parts := strings.Split(s, ".")
	version.Mayor, err = strconv.Atoi(parts[0])
	if err != nil {
		return
	}

	if len(parts) > 1 {
		version.Minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return
		}
	}

	if len(parts) > 2 {
		version.Patch, err = strconv.Atoi(parts[2])
	}
	return
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Mayor, v.Minor, v.Patch)
}

// Compare returns 1 if v version is higher than v2,
// 0 if they are equal and -1 if v version is lower than v2
func (v Version) Compare(v2 Version) int {
	if v.Mayor > v2.Mayor {
		return 1
	} else if v.Mayor < v2.Mayor {
		return -1
	}

	if v.Minor > v2.Minor {
		return 1
	} else if v.Minor < v2.Minor {
		return -1
	}

	if v.Patch > v2.Patch {
		return 1
	} else if v.Patch < v2.Patch {
		return -1
	}

	return 0
}

// TBLink stores a link to download Tor Browser with a certain locale for a certain platform
type TBLink struct {
	core.ResourceBase
	Locale       string         `json:"locale"`
	Platform     string         `json:"platform"`
	Version      Version        `json:"version"`
	Provider     string         `json:"provider"`
	FileName     string         `json:"file_name"`
	Link         string         `json:"link"`
	SigLink      string         `json:"sig_link"`
	CustomOid    *core.Hashkey  `json:"custom_oid"`
	CustomExpiry *time.Duration `json:"custom_expiry"`
}

// NewTBLink allocates and returns a new TBLink object.
func NewTBLink() *TBLink {
	tl := &TBLink{ResourceBase: *core.NewResourceBase()}
	tl.TestResult().State = core.StateFunctional
	tl.SetType(ResourceTypeTBLink)
	return tl
}

// IsPublic always returns true as all tor links are public
func (tl *TBLink) IsPublic() bool {
	return true
}

func (tl *TBLink) IsValid() bool {
	return true
}

func (tl *TBLink) Oid() core.Hashkey {
	if tl.CustomOid != nil {
		return *tl.CustomOid
	}
	return core.NewHashkey(tl.Link)
}

func (tl *TBLink) Uid() core.Hashkey {
	return tl.Oid()
}

func (tl *TBLink) Test() {
}

func (tl *TBLink) String() string {
	return tl.Link
}

// Expiry TBLinks that are older than a year, a newer version should have already being released
func (tl *TBLink) Expiry() time.Duration {
	if tl.CustomExpiry != nil {
		return *tl.CustomExpiry
	}
	return time.Duration(time.Hour * 24 * 365)
}

// Distributor set for this link
func (tl *TBLink) Distributor() string {
	return ""
}

func (tl *TBLink) downloadFile(filePath, link string) (string, error) {
	if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
		resp, err := http.Get(link)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
		bytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		err = ioutil.WriteFile(filePath, bytes, 0644)
		if err != nil {
			return "", err
		}
		return filePath, nil
	}
	return filePath, nil
}

func (t *TBLink) generateTorrent(file string, announces []string) (*metainfo.MetaInfo, error) {
	//info, err := metainfo.NewInfoFromFilePath(file, 5120)
	info, err := metainfo.NewInfoFromFilePath(file, 10240)
	if err != nil {
		return nil, fmt.Errorf("GenerateTorrent: %s", err)
	}
	info.Name = filepath.Base(file)
	var mi metainfo.MetaInfo
	mi.InfoBytes, err = bencode.EncodeBytes(info)
	if err != nil {
		return nil, fmt.Errorf("GenerateTorrent: %s", err)
	}
	switch len(announces) {
	case 0:
		// idk's Open Tracker inside I2P
		mi.Announce = "http://mb5ir7klpc2tj6ha3xhmrs3mseqvanauciuoiamx2mmzujvg67uq.b32.i2p/a"
	case 1:
		mi.Announce = announces[0]
	default:
		mi.AnnounceList = metainfo.AnnounceList{announces}
	}
	url, err := url.Parse("http://idk.i2p/torbrowser/" + filepath.Base(file))
	if err != nil {
		return nil, fmt.Errorf("GenerateTorrent: %s", err)
	}
	mi.URLList = []string{url.String()}
	clearurl, err := url.Parse("https://eyedeekay.github.io/torbrowser/" + filepath.Base(file))
	if err != nil {
		return nil, fmt.Errorf("GenerateTorrent: %s", err)
	}
	mi.URLList = append(mi.URLList, clearurl.String())
	return &mi, nil
}

func (tl *TBLink) GenerateFileMagnet(filePath string) (string, error) {
	filePath, err := tl.downloadFile(filePath, tl.Link)
	if err != nil {
		return "", err
	}
	mi, err := tl.generateTorrent(filePath, []string{})
	if err != nil {
		return "", err
	}
	return mi.Magnet("", mi.InfoHash()).String(), nil
}

func (tl *TBLink) GenerateSigMagnet(filePath string) (string, error) {
	filePath, err := tl.downloadFile(filePath, tl.SigLink)
	if err != nil {
		return "", err
	}
	mi, err := tl.generateTorrent(filePath, []string{})
	if err != nil {
		return "", err
	}
	return mi.Magnet("", mi.InfoHash()).String(), nil
}

// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"testing"
)

const (
	blocklistFile       = "./test_assets/blocklist"
	allowlistFile       = "./test_assets/allowlist"
	brokenBlocklistFile = "./test_assets/broken_blocklist"
	fp                  = "ABCDEF123456790"
	fp2                 = "123456790ABCDEF"
)

func TestEmptyBlockList(t *testing.T) {
	bl, err := newBlockList("", "")
	if err != nil {
		t.Fatal(err)
	}

	if len(bl.blocked) != 0 {
		t.Error("Unexepected blocked resources", bl.blocked)
	}

	if len(bl.allowed) != 0 {
		t.Error("Unexepected allowed resources", bl.allowed)
	}

	blockedIn := bl.blockedIn(fp)
	if len(blockedIn) != 0 {
		t.Error("Unexepected blocked in resource", blockedIn)
	}
}

func TestBlockAllowList(t *testing.T) {
	bl, err := newBlockList(blocklistFile, allowlistFile)
	if err != nil {
		t.Fatal(err)
	}

	blockedIn := bl.blockedIn(fp)
	for _, c := range []string{"aa", "dd"} {
		if !blockedIn[c] {
			t.Error(fp, "not blocked in", c)
		}
	}
	for _, c := range []string{"bb", "cc", "ff"} {
		if blockedIn[c] {
			t.Error(fp, "blocked in", c)
		}
	}
	if blockedIn["ee"] {
		t.Error("Blocklist is taking precedence on top of the allowlist and", fp, "is being blocked in ee")
	}

	blockedIn2 := bl.blockedIn(fp2)
	for _, c := range []string{"bb", "cc", "ee"} {
		if !blockedIn2[c] {
			t.Error(fp2, "not blocked in", c)
		}
	}
	for _, c := range []string{"aa", "dd", "ff"} {
		if blockedIn2[c] {
			t.Error(fp2, "blocked in", c)
		}
	}
}

func TestBlockList(t *testing.T) {
	bl, err := newBlockList(blocklistFile, "")
	if err != nil {
		t.Fatal(err)
	}

	blockedIn := bl.blockedIn(fp)
	for _, c := range []string{"aa", "ee"} {
		if !blockedIn[c] {
			t.Error(fp, "not blocked in", c)
		}
	}
	for _, c := range []string{"bb", "cc", "dd", "ff"} {
		if blockedIn[c] {
			t.Error(fp, "blocked in", c)
		}
	}

	blockedIn2 := bl.blockedIn(fp2)
	for _, c := range []string{"bb"} {
		if !blockedIn2[c] {
			t.Error(fp2, "not blocked in", c)
		}
	}
	for _, c := range []string{"aa", "cc", "dd", "ee", "ff"} {
		if blockedIn2[c] {
			t.Error(fp2, "blocked in", c)
		}
	}
}

func TestAllowList(t *testing.T) {
	bl, err := newBlockList("", allowlistFile)
	if err != nil {
		t.Fatal(err)
	}

	blockedIn := bl.blockedIn(fp)
	for _, c := range []string{"dd"} {
		if !blockedIn[c] {
			t.Error(fp, "not blocked in", c)
		}
	}
	for _, c := range []string{"aa", "bb", "cc", "ee", "ff"} {
		if blockedIn[c] {
			t.Error(fp, "blocked in", c)
		}
	}

	blockedIn2 := bl.blockedIn(fp2)
	for _, c := range []string{"cc", "ee"} {
		if !blockedIn2[c] {
			t.Error(fp2, "not blocked in", c)
		}
	}
	for _, c := range []string{"aa", "bb", "dd", "ff"} {
		if blockedIn2[c] {
			t.Error(fp2, "blocked in", c)
		}
	}
}

func TestIgnoreMalformedBlockListLines(t *testing.T) {
	bl, err := newBlockList(brokenBlocklistFile, "")
	if err != nil {
		t.Fatal(err)
	}

	blockedIn := bl.blockedIn(fp)
	for _, c := range []string{"aa"} {
		if !blockedIn[c] {
			t.Error(fp, "not blocked in", c)
		}
	}
	for _, c := range []string{"bb", "cc", "dd", "ee", "ff"} {
		if blockedIn[c] {
			t.Error(fp, "blocked in", c)
		}
	}

	blockedIn2 := bl.blockedIn(fp2)
	for _, c := range []string{"ee"} {
		if !blockedIn2[c] {
			t.Error(fp2, "not blocked in", c)
		}
	}
	for _, c := range []string{"aa", "bb", "cc", "dd", "ff"} {
		if blockedIn2[c] {
			t.Error(fp2, "blocked in", c)
		}
	}
}

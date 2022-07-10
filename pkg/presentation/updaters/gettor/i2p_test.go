package gettor

import (
	"testing"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
)

func TestGetReleases(t *testing.T) {
	i := newI2PProvider(&internal.I2P{})
	releases, err := i.getRelease()
	if err != nil {
		t.Errorf("failed to get releases: %s", err)
	}
	if releases == "" {
		t.Errorf("got no releases")
	}
}

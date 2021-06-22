package gettor

import (
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
	"testing"
)

const (
	platform = "win32"
)

func TestDeleteOldVersion(t *testing.T) {
	lastVersion := resources.Version{1, 0, 0}
	oldVersion := resources.Version{0, 1, 0}
	newLink := "new"
	oldLink := "old"
	dist := GettorDistributor{
		version: map[string]resources.Version{
			platform: lastVersion,
		},
		tblinks: TBLinkList{
			platform: {
				"en": {
					&resources.TBLink{
						Link:    oldLink,
						Version: oldVersion,
					},
				},
				"es": {
					&resources.TBLink{
						Link:    oldLink,
						Version: oldVersion,
					},
					&resources.TBLink{
						Link:    newLink,
						Version: lastVersion,
					},
					&resources.TBLink{
						Link:    oldLink + "1",
						Version: oldVersion,
					},
				},
			},
		},
	}

	dist.deleteOldVersions(platform)

	if len(dist.tblinks[platform]["es"]) != 1 {
		t.Fatal("Wrong size of tblinks: ", dist.tblinks[platform]["es"])
	}
	if dist.tblinks[platform]["es"][0].Link != newLink {
		t.Error("Unexpected tblink:", dist.tblinks[platform]["es"][0])
	}

	for lang := range dist.tblinks[platform] {
		for _, link := range dist.tblinks[platform][lang] {
			if link.Link != newLink {
				t.Error("Wrong link version of link:", link)
			}
		}
	}
}

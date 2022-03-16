package moat

import (
	"strings"
	"testing"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
)

var (
	config = internal.Config{
		Distributors: internal.Distributors{
			Moat: internal.MoatDistConfig{
				Resources:           []string{"dummy"},
				BuiltInBridgesTypes: []string{"snowflake"},
			},
		},
	}

	dummyResource = core.NewDummy(core.NewHashkey("oid"), core.NewHashkey("uid"))
)

const (
	circumventionMap = `
	{
		"cn": {
			"settings": [
				{"bridges": {"type": "snowflake", "source": "builtin"}}
			]
		},
		"fr": {
			"settings": [
				{"bridges": {"type": "dummy",     "source": "bridgedb"}},
				{"bridges": {"type": "snowflake", "source": "builtin"}}
			]
		}
	}`
)

func fetchBridges(url string) ([]string, error) {
	bridgeLines := []string{"snowflake 192.0.2.3:1 2B280B23E1107BB62ABFC40DDCC8824814F80A72"}
	return bridgeLines, nil
}

func initDistributor() *MoatDistributor {
	d := MoatDistributor{
		FetchBridges: fetchBridges,
	}
	d.Init(&config)
	d.collection["dummy"].Add(dummyResource)
	return &d
}

func TestCircumventionMap(t *testing.T) {
	d := initDistributor()
	defer d.Shutdown()

	err := d.LoadCircumventionMap(strings.NewReader(circumventionMap))
	if err != nil {
		t.Fatal("Can parse circumventionMap", err)
	}

	m := d.GetCircumventionMap()
	if len(m["cn"].Settings) != 1 {
		t.Fatal("Wrong length of 'cn' bridges")
	}
	if m["cn"].Settings[0].Bridges.Type != "snowflake" {
		t.Error("Wrong type of 'cn' bridge", m["cn"].Settings[0].Bridges.Type)
	}
}

func TestCircumventionSettings(t *testing.T) {
	d := initDistributor()
	defer d.Shutdown()

	err := d.LoadCircumventionMap(strings.NewReader(circumventionMap))
	if err != nil {
		t.Fatal("Can parse circumventionMap", err)
	}

	settings, err := d.GetCircumventionSettings("gb", []string{}, nil)
	if err != nil {
		t.Fatal("Can get circumvention settings for gb:", err)
	}
	if settings != nil {
		t.Error("Unexpected settins for 'gb'", settings)
	}

	settings, err = d.GetCircumventionSettings("cn", []string{}, nil)
	if err != nil {
		t.Fatal("Can get circumvention settings for cn:", err)
	}
	if settings == nil {
		t.Fatal("No settins for 'cn'")
	}
	if settings.Settings[0].Bridges.Type != "snowflake" {
		t.Error("Wrong type of 'cn' settings bridge", settings.Settings[0].Bridges.Type)
	}

	settings, err = d.GetCircumventionSettings("fr", []string{}, nil)
	if err != nil {
		t.Fatal("Can get circumvention settings for fr:", err)
	}
	if settings == nil {
		t.Fatal("No settins for 'fr'")
	}
	if settings.Settings[0].Bridges.Type != "dummy" {
		t.Error("Wrong type of 'fr' settings bridge", settings.Settings[0].Bridges.Type)
	}

	settings, err = d.GetCircumventionSettings("fr", []string{"snowflake"}, nil)
	if err != nil {
		t.Fatal("Can get circumvention settings for fr:", err)
	}
	if settings == nil {
		t.Fatal("No settins for 'fr'")
	}
	if settings.Settings[0].Bridges.Type != "snowflake" {
		t.Error("Now snowlfake type of 'fr' settings bridge", settings.Settings[0].Bridges.Type)
	}

	settings, err = d.GetCircumventionSettings("fr", []string{"snowflake", "dummy"}, nil)
	if err != nil {
		t.Fatal("Can get circumvention settings for fr:", err)
	}
	if settings == nil {
		t.Fatal("No settins for 'fr'")
	}
	if settings.Settings[0].Bridges.Type != "dummy" {
		t.Error("Wrong type of 'fr' settings bridge", settings.Settings[0].Bridges.Type)
	}
}

func TestBuiltInBridges(t *testing.T) {
	d := initDistributor()
	defer d.Shutdown()

	bridges := d.GetBuiltInBridges([]string{})
	lines, ok := bridges["snowflake"]
	if !ok {
		t.Fatal("No snowflake bridges found")
	}
	if len(lines) != 1 {
		t.Fatal("Wrong lines", lines)
	}

	bridges = d.GetBuiltInBridges([]string{"dummy"})
	lines, ok = bridges["snowflake"]
	if ok {
		t.Fatal("snowflake bridges found")
	}

	bridges = d.GetBuiltInBridges([]string{"snowflake"})
	lines, ok = bridges["snowflake"]
	if !ok {
		t.Fatal("No snowflake bridges found")
	}
}

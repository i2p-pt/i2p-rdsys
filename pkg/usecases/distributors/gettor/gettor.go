package gettor

import (
	"bufio"
	"io"
	"log"
	"strings"
	"sync"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/delivery"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/delivery/mechanisms"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

const (
	DistName = "gettor"

	CommandHelp  = "help"
	CommandLinks = "links"
)

type GettorDistributor struct {
	ipc      delivery.Mechanism
	wg       sync.WaitGroup
	shutdown chan bool
	tblinks  TBLinkList

	// latest version of Tor Browser per platform
	version map[string]resources.Version

	// locales map a lowercase locale to its correctly cased locale
	locales map[string]string
}

// TBLinkList are indexed first by platform and last by locale
type TBLinkList map[string]map[string][]*resources.TBLink

type Command struct {
	Locale   string
	Platform string
	Command  string
}

func (d *GettorDistributor) GetLinks(platform, locale string) []*resources.TBLink {
	return d.tblinks[platform][locale]
}

func (d *GettorDistributor) ParseCommand(body io.Reader) *Command {
	command := Command{
		Locale:   "",
		Platform: "",
		Command:  "",
	}

	scanner := bufio.NewScanner(body)
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() {
		if command.Locale != "" && (command.Platform != "" || command.Command != "") {
			break
		}

		word := strings.ToLower(scanner.Text())
		if word == "help" {
			command.Command = CommandHelp
			continue
		}

		if command.Locale == "" {
			locale, exists := d.locales[word]
			if exists {
				command.Locale = locale
				continue
			}
		}

		if command.Platform == "" {
			_, exists := d.tblinks[word]
			if exists {
				command.Platform = word
				continue
			}
		}
	}

	if command.Command == "" {
		if command.Platform == "" {
			command.Command = CommandHelp
		} else {
			command.Command = CommandLinks
		}
	}

	if command.Locale == "" {
		command.Locale = "en-US"
	}

	return &command
}

func (d *GettorDistributor) SupportedPlatforms() []string {
	platforms := make([]string, 0, len(d.tblinks))
	for platform := range d.tblinks {
		platforms = append(platforms, platform)
	}
	return platforms
}

func (d *GettorDistributor) SupportedLocales() []string {
	locales := make([]string, 0, len(d.locales))
	for locale := range d.locales {
		locales = append(locales, locale)
	}
	return locales
}

// housekeeping listens to updates from the backend resources
func (d *GettorDistributor) housekeeping(rStream chan *core.ResourceDiff) {
	defer d.wg.Done()
	defer close(rStream)
	defer d.ipc.StopStream()

	for {
		select {
		case diff := <-rStream:
			d.applyDiff(diff)
		case <-d.shutdown:
			log.Printf("Shutting down housekeeping.")
			return
		}
	}
}

func (d *GettorDistributor) Init(cfg *internal.Config) {
	d.shutdown = make(chan bool)
	d.tblinks = make(TBLinkList)
	d.locales = make(map[string]string)
	d.version = make(map[string]resources.Version)

	d.ipc = mechanisms.NewHttpsIpc(
		"http://"+cfg.Backend.WebApi.ApiAddress+cfg.Backend.ResourceStreamEndpoint,
		"GET",
		cfg.Backend.ApiTokens[DistName])
	rStream := make(chan *core.ResourceDiff)
	req := core.ResourceRequest{
		RequestOrigin: DistName,
		ResourceTypes: cfg.Distributors.Gettor.Resources,
		Receiver:      rStream,
	}
	d.ipc.StartStream(&req)

	d.wg.Add(1)
	go d.housekeeping(rStream)
}

func (d *GettorDistributor) Shutdown() {
	close(d.shutdown)
	d.wg.Wait()
}

// applyDiff to tblinks. Ignore changes, links should not change, just appear new or be gone
func (d *GettorDistributor) applyDiff(diff *core.ResourceDiff) {
	needsCleanUp := map[string]struct{}{}
	for rType, resourceQueue := range diff.New {
		if rType != "tblink" {
			continue
		}
		for _, r := range resourceQueue {
			link, ok := r.(*resources.TBLink)
			if !ok {
				log.Println("Not valid tblink resource", r)
				continue
			}
			version, ok := d.version[link.Platform]
			if ok {
				switch version.Compare(link.Version) {
				case 1:
					// ignore resources with old versions
					continue
				case -1:
					d.version[link.Platform] = link.Version
					needsCleanUp[link.Platform] = struct{}{}
				}
			} else {
				d.version[link.Platform] = link.Version
			}

			_, ok = d.tblinks[link.Platform]
			if !ok {
				d.tblinks[link.Platform] = make(map[string][]*resources.TBLink)
			}
			d.tblinks[link.Platform][link.Locale] = append(d.tblinks[link.Platform][link.Locale], link)

			d.locales[strings.ToLower(link.Locale)] = link.Locale
		}
	}

	for rType, resourceQueue := range diff.Gone {
		if rType != "tblink" {
			continue
		}
		for _, r := range resourceQueue {
			link, ok := r.(*resources.TBLink)
			if !ok {
				log.Println("Not valid tblink resource", r)
				continue
			}
			_, ok = d.tblinks[link.Platform]
			if !ok {
				continue
			}
			for i, l := range d.tblinks[link.Platform][link.Locale] {
				if l.Link == link.Link {
					linklist := d.tblinks[link.Platform][link.Locale]
					d.tblinks[link.Platform][link.Locale] = append(linklist[:i], linklist[i+1:]...)
					break
				}
			}
		}
	}

	for platform := range needsCleanUp {
		d.deleteOldVersions(platform)
	}
}

func (d *GettorDistributor) deleteOldVersions(platform string) {
	locales := d.tblinks[platform]
	for locale, res := range locales {
		newResources := []*resources.TBLink{}
		for _, r := range res {
			if d.version[platform].Compare(r.Version) == 0 {
				newResources = append(newResources, r)
			}
		}

		if len(newResources) == 0 {
			delete(d.tblinks[platform], locale)
		} else {
			d.tblinks[platform][locale] = newResources
		}
	}
	if len(d.tblinks[platform]) == 0 {
		delete(d.tblinks, platform)
	}
}

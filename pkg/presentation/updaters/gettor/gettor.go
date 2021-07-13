package gettor

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/updaters/gettor"
)

const (
	downloadsURL    = "https://aus1.torproject.org/torbrowser/update_3/release/downloads.json"
	updateFrequency = time.Hour
)

// updatedLinks keeps the links to be sent to the backend
// we want to keep them as a global variable to be able to retry if the backend fails
var updatedLinks = []*resources.TBLink{}

type uploadFileFunc func(binaryPath string, sigPath string, locale string) *resources.TBLink
type provider interface {
	needsUpdate(platform string, version resources.Version) bool
	newRelease(platform string, version resources.Version) uploadFileFunc
}

type downloadsLinks struct {
	Version   string                                  `json:"version"`
	Downloads map[string]map[string]map[string]string `json:"downloads"`
}

func InitUpdater(cfg *internal.Config) {
	updater := &gettor.GettorUpdater{}
	updater.Init(cfg)

	stop := make(chan struct{})
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT)
	signal.Notify(signalChan, syscall.SIGTERM)
	go func() {
		<-signalChan
		log.Printf("Caught SIGINT.")
		updater.Shutdown()
		close(stop)
	}()

	gh := newGithubProvider(&cfg.Updaters.Gettor.Github)
	providers := []provider{gh}

	updateIfNeeded(updater, providers)
	for {
		select {
		case <-stop:
			return
		case <-time.After(updateFrequency):
			updateIfNeeded(updater, providers)
		}
	}
}

func updateIfNeeded(updater *gettor.GettorUpdater, providers []provider) {
	downloads, version, err := getDownloadLinks()
	if err != nil {
		log.Println("Error fetching downloads.json:", err)
		return
	}

	tmpDir, err := ioutil.TempDir("", "gettor-")
	if err != nil {
		log.Println("Can't create temporary file:", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	for platform, locales := range downloads.Downloads {
		uploadFuncs := []uploadFileFunc{}
		for _, p := range providers {
			if p.needsUpdate(platform, version) {
				fn := p.newRelease(platform, version)
				if fn != nil {
					uploadFuncs = append(uploadFuncs, fn)
				}
			}
		}
		if len(uploadFuncs) == 0 {
			continue
		}

		for locale, assets := range locales {
			log.Println("Uploading to distributors", assets["binary"])
			binaryPath, err := getAsset(assets["binary"], tmpDir)
			if err != nil {
				log.Println("Error getting asset:", err)
				continue
			}
			sigPath, err := getAsset(assets["sig"], tmpDir)
			if err != nil {
				log.Println("Error getting asset:", err)
				continue
			}

			for _, fn := range uploadFuncs {
				link := fn(binaryPath, sigPath, locale)
				if link != nil {
					updatedLinks = append(updatedLinks, link)
				}
			}

			os.Remove(binaryPath)
			os.Remove(sigPath)
		}

		if len(updatedLinks) == 0 {
			return
		}

		err = updater.AddLinks(updatedLinks)
		if err != nil {
			log.Println("Error sending links to the backend:", err)
		} else {
			log.Println("Updated links for", platform, version.String(), "in the backend")
			updatedLinks = nil
		}
	}
}

func getAsset(url string, tmpDir string) (filePath string, err error) {
	segments := strings.Split(url, "/")
	fileName := segments[len(segments)-1]
	filePath = path.Join(tmpDir, fileName)
	file, err := os.Create(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_, err = io.Copy(file, resp.Body)
	return
}

func getDownloadLinks() (downloads downloadsLinks, version resources.Version, err error) {
	resp, err := http.Get(downloadsURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	d := json.NewDecoder(resp.Body)
	err = d.Decode(&downloads)
	if err != nil {
		return
	}

	version, err = resources.Str2Version(downloads.Version)
	return
}
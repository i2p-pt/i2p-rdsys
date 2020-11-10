package https

import (
	"fmt"
	"hash/crc64"
	"log"
	"net/http"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/presentation/distributors/common"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/distributors/https"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v2"

	"github.com/nicksnyder/go-i18n/v2/i18n"
)

var bundle *i18n.Bundle

var dist *https.HttpsDistributor

// mapRequestToHashkey maps the given HTTP request to a hash key.  It does so
// by taking the /16 of the client's IP address.  For example, if the client's
// address is 1.2.3.4, the function turns it into 1.2., computes its CRC64, and
// returns the resulting hash key.
func mapRequestToHashkey(r *http.Request) core.Hashkey {

	i := 0
	for numDots := 0; i < len(r.RemoteAddr) && numDots < 2; i++ {
		if r.RemoteAddr[i] == '.' {
			numDots++
		}
	}
	slash16 := r.RemoteAddr[:i]
	log.Printf("Using address prefix %q as hash key.", slash16)
	table := crc64.MakeTable(resources.Crc64Polynomial)

	return core.Hashkey(crc64.Checksum([]byte(slash16), table))
}

// RequestHandler handles requests for /.
func RequestHandler(w http.ResponseWriter, r *http.Request) {

	lang := r.FormValue("lang")
	accept := r.Header.Get("Accept-Language")
	localizer := i18n.NewLocalizer(bundle, lang, accept)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	resources, err := dist.RequestBridges(mapRequestToHashkey(r))
	if err != nil {
		fmt.Fprintf(w, err.Error())
	}

	pageTitle := localizer.MustLocalize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    "PageTitle",
			Other: "Request Tor Bridges",
		},
	})

	voilaBridges := localizer.MustLocalize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    "VoilaBridges",
			One:   "Here is your bridge:",
			Other: "Here are your {{.Count}} bridges:",
		},
		PluralCount: len(resources),
		TemplateData: map[string]interface{}{
			"Count": len(resources),
		},
	})

	// TODO: We have to get the user's preferred language; e.g. by taking a
	// look at the Accept-Language header.
	err = indexPage.Execute(w, map[string]interface{}{
		"Lang":         "en",
		"Title":        pageTitle,
		"VoilaBridges": voilaBridges,
		"Bridges":      resources,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("localisation failed: %s", err),
			http.StatusInternalServerError)
	}
}

// InitFrontend is the entry point to HTTPS's Web frontend.  It spins up the
// Web server and then waits until it receives a SIGINT.
func InitFrontend(cfg *internal.Config) {

	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)
	//bundle.MustLoadMessageFile("active.es.toml")

	// TODO: create function that parses locations directory from config and
	// loads all translations.
	_, err := bundle.LoadMessageFile("locales/active.de.yaml")
	if err != nil {
		log.Printf("failed to load translations: %s", err)
	}

	dist = &https.HttpsDistributor{}
	handlers := map[string]http.HandlerFunc{
		"/": http.HandlerFunc(RequestHandler),
	}

	common.StartWebServer(
		&cfg.Distributors.Https.WebApi,
		cfg,
		dist,
		handlers,
	)
}

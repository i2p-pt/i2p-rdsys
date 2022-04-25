// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package moat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"gitlab.torproject.org/tpo/anti-censorship/geoip"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/presentation/distributors/common"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/distributors/moat"
)

var (
	dist    *moat.MoatDistributor
	geoipdb *geoip.Geoip
)

type jsonError struct {
	Errors []jsonErrorEntry `json:"errors"`
}

type jsonErrorEntry struct {
	Code   int    `json:"code"`
	Detail string `json:"detail"`
}

var (
	invalidRequest = jsonError{[]jsonErrorEntry{{
		Code:   400,
		Detail: "Not valid request",
	}}}
	countryNotFound = jsonError{[]jsonErrorEntry{{
		Code:   406,
		Detail: "Could not find country code for circumvention settings",
	}}}
	transportNotFound = jsonError{[]jsonErrorEntry{{
		Code:   404,
		Detail: "No provided transport is available for this country",
	}}}
)

// InitFrontend is the entry point to HTTPS's Web frontend.  It spins up the
// Web server and then waits until it receives a SIGINT.
func InitFrontend(cfg *internal.Config) {
	dist = &moat.MoatDistributor{
		FetchBridges: fetchBridges,
	}
	err := loadCircumventionFile(cfg.Distributors.Moat.CircumventionMap, dist.LoadCircumventionMap)
	if err != nil {
		log.Fatalf("Can't load circumvention map %s: %v", cfg.Distributors.Moat.CircumventionMap, err)
	}
	err = loadCircumventionFile(cfg.Distributors.Moat.CircumventionDefaults, dist.LoadCircumventionDefaults)
	if err != nil {
		log.Fatalf("Can't load circumvention defaults %s: %v", cfg.Distributors.Moat.CircumventionDefaults, err)
	}

	geoipdb, err = geoip.New(cfg.Distributors.Moat.GeoipDB, cfg.Distributors.Moat.Geoip6DB)
	if err != nil {
		log.Fatal("Can't load geoip databases", cfg.Distributors.Moat.GeoipDB, cfg.Distributors.Moat.Geoip6DB, ":", err)
	}

	handlers := map[string]http.HandlerFunc{
		"/moat/circumvention/map":            http.HandlerFunc(circumventionMapHandler),
		"/moat/circumvention/countries":      http.HandlerFunc(countriesHandler),
		"/moat/circumvention/settings":       http.HandlerFunc(circumventionSettingsHandler),
		"/moat/circumvention/builtin":        http.HandlerFunc(builtinHandler),
		"/moat/circumvention/defaults":       http.HandlerFunc(circumventionDefaultsHandler),
		"/meek/moat/circumvention/map":       http.HandlerFunc(circumventionMapHandler),
		"/meek/moat/circumvention/countries": http.HandlerFunc(countriesHandler),
		"/meek/moat/circumvention/settings":  http.HandlerFunc(circumventionSettingsHandler),
		"/meek/moat/circumvention/builtin":   http.HandlerFunc(builtinHandler),
		"/meek/moat/circumvention/defaults":  http.HandlerFunc(circumventionDefaultsHandler),
	}

	common.StartWebServer(
		&cfg.Distributors.Moat.WebApi,
		cfg,
		dist,
		handlers,
	)
}

func loadCircumventionFile(path string, loadFn func(r io.Reader) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return loadFn(f)
}

func circumventionMapHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/json; charset=utf-8")
	m := dist.GetCircumventionMap()
	enc := json.NewEncoder(w)
	err := enc.Encode(m)
	if err != nil {
		log.Println("Error encoding circumvention map:", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}
func countriesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/json; charset=utf-8")
	m := dist.GetCircumventionMap()
	countries := make([]string, 0, len(m))
	for k := range m {
		countries = append(countries, k)
	}

	enc := json.NewEncoder(w)
	err := enc.Encode(countries)
	if err != nil {
		log.Println("Error encoding countries list:", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

type circumventionSettingsRequest struct {
	Country    string   `json:"country"`
	Transports []string `json:"transports"`
}

func circumventionSettingsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/json; charset=utf-8")
	enc := json.NewEncoder(w)

	var request circumventionSettingsRequest
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&request)
	if err != nil && !errors.Is(err, io.EOF) {
		log.Println("Error decoding circumvention settings request:", err)
		err = enc.Encode(invalidRequest)
		if err != nil {
			log.Println("Error encoding jsonError:", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	ip := ipFromRequest(r)
	if request.Country == "" {
		request.Country = countryFromIP(ip)
		if request.Country == "" {
			log.Println("Could not find country code for cicrumvention settings")
			err = enc.Encode(countryNotFound)
			if err != nil {
				log.Println("Error encoding jsonError:", err)
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
	}

	s, err := dist.GetCircumventionSettings(request.Country, request.Transports, ip)
	if err != nil {
		if errors.Is(err, moat.NoTransportError) {
			err = enc.Encode(transportNotFound)
			if err != nil {
				log.Println("Error encoding jsonError:", err)
				w.WriteHeader(http.StatusInternalServerError)
			}
		} else {
			log.Println("Error getting circumvention settings:", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	if s == nil {
		w.Write([]byte("{}"))
		return
	}

	err = enc.Encode(s)
	if err != nil {
		log.Println("Error encoding circumvention settings:", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func circumventionDefaultsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/json; charset=utf-8")
	enc := json.NewEncoder(w)

	var request transportsRequest
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&request)
	if err != nil && !errors.Is(err, io.EOF) {
		log.Println("Error decoding circumvention defaults request:", err)
		err = enc.Encode(invalidRequest)
		if err != nil {
			log.Println("Error encoding jsonError:", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	ip := ipFromRequest(r)
	s, err := dist.GetCircumventionDefaults(request.Transports, ip)
	if err != nil {
		if errors.Is(err, moat.NoTransportError) {
			err = enc.Encode(transportNotFound)
			if err != nil {
				log.Println("Error encoding jsonError:", err)
				w.WriteHeader(http.StatusInternalServerError)
			}
		} else {
			log.Println("Error getting circumvention defaults:", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	if s == nil {
		w.Write([]byte("{}"))
		return
	}

	err = enc.Encode(s)
	if err != nil {
		log.Println("Error encoding circumvention defaults:", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func ipFromRequest(r *http.Request) net.IP {
	header := r.Header.Get("X-Forwarded-For")
	forwarded := strings.Split(header, ",")
	var ip net.IP
	for _, f := range forwarded {
		ip = net.ParseIP(f)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() { // FIXME: go1.17 || ip.IsPrivate()
			ip = nil
			continue
		}
		break
	}

	if ip == nil {
		// if no X-Forwarded-For header let's take the IP from the request
		ipStr := strings.Split(r.RemoteAddr, ":")[0]
		ip = net.ParseIP(ipStr)
		if ip == nil {
			return ip
		}
	}

	return ip
}

func countryFromIP(ip net.IP) string {
	country, ok := geoipdb.GetCountryByAddr(ip)
	if !ok {
		return ""
	}
	return strings.ToLower(country)
}

type transportsRequest struct {
	Transports []string `json:"transports"`
}

func builtinHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/json; charset=utf-8")
	enc := json.NewEncoder(w)

	var request transportsRequest
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&request)
	if err != nil && !errors.Is(err, io.EOF) {
		log.Println("Error decoding builtin request:", err)
		err = enc.Encode(invalidRequest)
		if err != nil {
			log.Println("Error encoding jsonError:", err)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	bb := dist.GetBuiltInBridges(request.Transports)
	err = enc.Encode(bb)
	if err != nil {
		log.Println("Error encoding builtin bridges:", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func fetchBridges(url string) ([]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	body = bytes.TrimSpace(body)
	return strings.Split(string(body), "\n"), nil
}

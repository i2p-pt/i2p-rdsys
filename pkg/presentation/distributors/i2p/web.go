// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package i2phttps

import (
	"fmt"
	"log"
	"net/http"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/presentation/distributors/common"
	i2phttps "gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/distributors/i2p"

	sam "github.com/eyedeekay/sam3/helper"
)

var dist *i2phttps.I2PHttpsDistributor

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

	return core.NewHashkey(slash16)
}

// RequestHandler handles requests for /.
func RequestHandler(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	resources, err := dist.RequestBridges(mapRequestToHashkey(r))
	if err != nil {
		fmt.Fprintf(w, err.Error())
	} else {
		fmt.Fprintf(w, "Your %s bridge(s):<br>", resources[0].Type())
		for _, res := range resources {
			fmt.Fprintf(w, fmt.Sprintf("<tt>%s</tt><br>", res.String()))
		}
	}
}

// InitFrontend is the entry point to HTTPS's Web frontend.  It spins up the
// Web server and then waits until it receives a SIGINT.
func InitFrontend(cfg *internal.Config) {

	dist = &i2phttps.I2PHttpsDistributor{}
	handlers := map[string]http.HandlerFunc{
		"/": http.HandlerFunc(RequestHandler),
	}

	listener, err := sam.I2PListener(cfg.Distributors.I2P.WebApi.ApiAddress, "127.0.0.1:7656", cfg.Distributors.I2P.WebApi.ApiAddress)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Listening on", listener.Addr())
	log.Println("Dumping config...")
	log.Println(cfg)
	log.Println("...done.")

	common.ListenWebServer(
		listener,
		&cfg.Distributors.I2P.WebApi,
		cfg,
		dist,
		handlers,
	)
}

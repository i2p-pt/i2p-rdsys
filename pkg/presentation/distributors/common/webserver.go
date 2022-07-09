// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/distributors"
)

// StartWebServer helps distributor frontends start a Web server and configure
// handlers.  This function does not return until it receives a SIGINT or
// SIGTERM.  When that happens, the function calls the distributor's Shutdown
// method and shuts down the Web server.
func StartWebServer(apiCfg *internal.WebApiConfig, distCfg *internal.Config,
	dist distributors.Distributor, handlers map[string]http.HandlerFunc) {
	listener, err := net.Listen("tcp", apiCfg.ApiAddress)
	if err != nil {
		log.Fatalf("Error listening on %s: %s", apiCfg.ApiAddress, err)
	}
	ListenWebServer(listener, apiCfg, distCfg, dist, handlers)
}

// ListenWebServer helps distributor frontends start a Web server and configure
// handlers.  This function does not return until it receives a SIGINT or
// SIGTERM.  When that happens, the function calls the distributor's Shutdown
// method and shuts down the Web server. It is identical to the StartWebServer
// except that it takes a net.Listener instead of a string address.
func ListenWebServer(listener net.Listener, apiCfg *internal.WebApiConfig, distCfg *internal.Config,
	dist distributors.Distributor, handlers map[string]http.HandlerFunc) {

	var srv http.Server
	dist.Init(distCfg)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT)
	signal.Notify(signalChan, syscall.SIGTERM)
	go func() {
		<-signalChan
		log.Printf("Caught SIGINT.")
		dist.Shutdown()

		log.Printf("Shutting down Web API.")
		// Give our Web server five seconds to shut down.
		t := time.Now().Add(5 * time.Second)
		ctx, cancel := context.WithDeadline(context.Background(), t)
		defer cancel()
		err := srv.Shutdown(ctx)
		if err != nil {
			log.Printf("Error shutting down Web API: %s", err)
		}
	}()

	mux := http.NewServeMux()
	for endpoint, handlerFunc := range handlers {
		mux.Handle(endpoint, handlerFunc)
	}
	srv.Handler = mux

	// srv.Addr = cfg.Distributors.Salmon.ApiAddress
	srv.Addr = apiCfg.ApiAddress
	log.Printf("Starting Web server at %s.", srv.Addr)
	//os.Exit(0)
	var err error
	if apiCfg.KeyFile != "" && apiCfg.CertFile != "" {
		err = srv.ServeTLS(
			listener,
			apiCfg.CertFile,
			apiCfg.KeyFile,
		)
	} else {
		err = srv.Serve(listener)
	}
	if err != nil {
		log.Printf("Web API shut down: %s", err)
	}
}

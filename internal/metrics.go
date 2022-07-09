// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

const (
	PrometheusNamespace = "rdsys_backend"
)

type Metrics struct {
	TestedResources           *prometheus.GaugeVec
	DistributingNonFunctional prometheus.Gauge
	IgnoringBridgeDescriptors prometheus.Gauge
	Resources                 *prometheus.GaugeVec
	DistributorResources      *prometheus.GaugeVec
	Requests                  *prometheus.CounterVec
}

// InitMetrics initialises our Prometheus metrics.
func InitMetrics() *Metrics {

	metrics := &Metrics{}

	metrics.TestedResources = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: PrometheusNamespace,
			Name:      "tested_resources",
			Help:      "The fraction of resources that are currently tested",
		},
		[]string{"type", "status"},
	)

	metrics.DistributingNonFunctional = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: PrometheusNamespace,
			Name:      "distributing_non_functional_resources",
			Help:      "If rdsys is distribution non functional bridges",
		},
	)

	metrics.IgnoringBridgeDescriptors = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: PrometheusNamespace,
			Name:      "ignoring_bridge_descriptors",
			Help:      "If the bridge descriptors are being ignored because of failures (high ratio of non-running)",
		},
	)

	metrics.Resources = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: PrometheusNamespace,
			Name:      "resources",
			Help:      "The number of resources we have",
		},
		[]string{"type"},
	)

	metrics.DistributorResources = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: PrometheusNamespace,
			Name:      "distributor_resources",
			Help:      "The number of resources we have per distributor",
		},
		[]string{"distributor", "type"},
	)

	metrics.Requests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: PrometheusNamespace,
			Name:      "requests_total",
			Help:      "The number of API requests",
		},
		[]string{"target"},
	)

	return metrics
}

func (m *Metrics) updateDistributors(cfg *Config, rcol *core.BackendResources) {
	file, err := os.OpenFile(cfg.Backend.AssignmentsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println("Can't open assignments file", cfg.Backend.AssignmentsFile, err)
		return
	}
	defer file.Close()

	fmt.Fprintln(file, "bridge-pool-assignment", time.Now().UTC().Format("2006-01-02 15:04:05"))
	for distributor := range cfg.Backend.DistProportions {
		for transport := range cfg.Backend.Resources {
			rs := rcol.Get(distributor, transport)
			for _, resource := range rs {
				transport, ok := resource.(*resources.Transport)
				if ok {
					info := bridgeInfo(transport.BridgeBase)
					fmt.Fprintln(file, transport.Fingerprint, distributor, "transport="+transport.Type(), info)
					continue
				}

				bridge, ok := resource.(*resources.Bridge)
				if ok {
					info := bridgeInfo(bridge.BridgeBase)
					fmt.Fprintln(file, bridge.Fingerprint, distributor, info)
				}
			}

			m.DistributorResources.
				With(prometheus.Labels{"distributor": distributor, "type": transport}).
				Set(float64(len(rs)))
		}
	}

}

func bridgeInfo(bridge resources.BridgeBase) string {
	ip := map[uint16]struct{}{}
	ipAddr := net.ParseIP(bridge.Address.String())
	if ipAddr == nil {
		ipAddr = net.ParseIP("127.0.0.1")
	}

	if ipAddr.To4() != nil {
		ip[4] = struct{}{}
	} else {
		ip[6] = struct{}{}
	}

	for _, address := range bridge.ORAddresses {
		ip[address.IPVersion] = struct{}{}
	}

	versions := make([]string, 0, len(ip))
	for version := range ip {
		versions = append(versions, strconv.Itoa(int(version)))
	}

	info := []string{"ip=" + strings.Join(versions, ",")}
	if bridge.Port == 443 {
		info = append(info, "port=443")
	}

	blockedIn := bridge.BlockedIn()
	if len(blockedIn) != 0 {
		countries := make([]string, 0, len(blockedIn))
		for k := range blockedIn {
			countries = append(countries, k)
		}

		info = append(info, "blocklist="+strings.Join(countries, ","))
	}

	return strings.Join(info, " ")
}

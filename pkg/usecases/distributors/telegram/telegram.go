// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/core"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/delivery"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/delivery/mechanisms"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

const (
	DistName = "telegram"
)

var (
	bridgeRequestsCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "telegram_bridges_request_total",
		Help: "The total number of bridge requests",
	},
		[]string{"pool", "status"},
	)
)

type metricsData struct {
	hashKey core.Hashkey
	pool    string
	err     error
}

type TelegramDistributor struct {
	oldHashring *core.Hashring
	newHashring *core.Hashring
	cfg         *internal.TelegramDistConfig
	ipc         delivery.Mechanism
	wg          sync.WaitGroup
	shutdown    chan bool

	metricsChan chan<- metricsData
}

func (d *TelegramDistributor) GetResources(id int64) []core.Resource {
	now := time.Now().Unix() / (60 * 60)
	period := now / int64(d.cfg.RotationPeriodHours)
	hashKey := core.NewHashkey(fmt.Sprintf("%d-%d", id, period))

	md := metricsData{hashKey: hashKey}

	resources, err := d.newHashring.GetMany(hashKey, d.cfg.NumBridgesPerRequest)
	if err != nil {
		log.Println("Error getting resources from the hashring:", err)
		md.err = err
	}

	md.pool = "new"
	if id < d.cfg.MinUserID {
		md.pool = "old"
		oldResources, err := d.oldHashring.GetMany(hashKey, d.cfg.NumBridgesPerRequest)
		if err != nil {
			log.Println("Error getting resources from the old hashring:", err)
			md.err = err
		}
		resources = append(oldResources, resources...)
	}

	d.metricsChan <- md
	return resources
}

// housekeeping listens to updates from the backend resources
func (d *TelegramDistributor) housekeeping(rStream chan *core.ResourceDiff) {
	defer d.wg.Done()
	defer close(rStream)
	defer d.ipc.StopStream()

	for {
		select {
		case diff := <-rStream:
			d.oldHashring.ApplyDiff(diff)
		case <-d.shutdown:
			log.Printf("Shutting down housekeeping.")
			return
		}
	}
}

func (d *TelegramDistributor) Init(cfg *internal.Config) {
	d.cfg = &cfg.Distributors.Telegram
	d.shutdown = make(chan bool)
	d.oldHashring = core.NewHashring()
	d.newHashring = core.NewHashring()

	metricsChan := make(chan metricsData)
	d.metricsChan = metricsChan
	go metricsUpdater(metricsChan, cfg.Distributors.Telegram.RotationPeriodHours)

	log.Printf("Initialising resource stream.")
	d.ipc = mechanisms.NewHttpsIpc(
		"http://"+cfg.Backend.WebApi.ApiAddress+cfg.Backend.ResourceStreamEndpoint,
		"GET",
		cfg.Backend.ApiTokens[DistName])
	rStream := make(chan *core.ResourceDiff)
	req := core.ResourceRequest{
		RequestOrigin: DistName,
		ResourceTypes: []string{d.cfg.Resource},
		Receiver:      rStream,
	}
	d.ipc.StartStream(&req)

	d.wg.Add(1)
	go d.housekeeping(rStream)
}

func (d *TelegramDistributor) Shutdown() {
	log.Printf("Shutting down %s distributor.", DistName)

	close(d.metricsChan)
	close(d.shutdown)
	d.wg.Wait()
}

func (d *TelegramDistributor) LoadNewBridges(r io.Reader) error {
	hashring := core.NewHashring()

	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		bridgeParts := strings.Split(scanner.Text(), " ")
		if bridgeParts[0] != d.cfg.Resource {
			return fmt.Errorf("Not valid bridge type %s", bridgeParts[0])
		}

		var bridge resources.Transport
		bridge.RType = bridgeParts[0]
		bridge.Fingerprint = bridgeParts[2]

		addrParts := strings.Split(bridgeParts[1], ":")
		if len(addrParts) != 2 {
			return fmt.Errorf("Malformed address %s", bridgeParts[1])
		}
		addr, err := net.ResolveIPAddr("", addrParts[0])
		if err != nil {
			return err
		}
		bridge.Address = resources.IPAddr{IPAddr: *addr}
		port, err := strconv.Atoi(addrParts[1])
		if err != nil {
			return fmt.Errorf("Can't convert port to integer: %s", err)
		}
		bridge.Port = uint16(port)

		bridge.Parameters = make(map[string]string)
		for _, param := range bridgeParts[3:] {
			paramParts := strings.Split(param, "=")
			if len(paramParts) != 2 {
				return fmt.Errorf("Malformed param %s", param)
			}
			bridge.Parameters[paramParts[0]] = paramParts[1]
		}

		hashring.Add(&bridge)
	}

	d.newHashring = hashring
	return nil
}

func metricsUpdater(ch <-chan metricsData, rotationPeriodHours int) {
	requestHashKeys := make(map[core.Hashkey]time.Time)
	lastCleanup := time.Now()

	for md := range ch {
		status := "fresh"
		keepDate := time.Now().Add(-time.Hour * time.Duration(rotationPeriodHours))
		if date, ok := requestHashKeys[md.hashKey]; ok && date.After(keepDate) {
			status = "cached"
		} else {
			requestHashKeys[md.hashKey] = time.Now()
		}
		if md.err != nil {
			status = "error"
		}
		bridgeRequestsCount.WithLabelValues(md.pool, status).Inc()

		if lastCleanup.Before(keepDate) {
			for hk, t := range requestHashKeys {
				if t.Before(keepDate) {
					delete(requestHashKeys, hk)
				}
			}
		}
	}
}

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

type TelegramDistributor struct {
	oldHashring *core.Hashring
	newHashring *core.Hashring
	cfg         *internal.TelegramDistConfig
	ipc         delivery.Mechanism
	wg          sync.WaitGroup
	shutdown    chan bool

	bridgeRequestsCount *prometheus.CounterVec
	requestHashKeys     map[core.Hashkey]time.Time
}

func (d *TelegramDistributor) GetResources(id int64) []core.Resource {
	hashring := d.oldHashring
	pool := "old"
	if d.cfg.MinUserID != 0 && id > d.cfg.MinUserID {
		hashring = d.newHashring
		pool = "new"
	}

	now := time.Now().Unix() / (60 * 60)
	period := now / int64(d.cfg.RotationPeriodHours)
	hashKey := core.NewHashkey(fmt.Sprintf("%d-%d", id, period))

	status := "fresh"
	if _, ok := d.requestHashKeys[hashKey]; ok {
		status = "cached"
	}
	d.requestHashKeys[hashKey] = time.Now()

	resources, err := hashring.GetMany(hashKey, d.cfg.NumBridgesPerRequest)
	if err != nil {
		log.Println("Error getting resources from the hashring:", err)
		status = "error"
	}

	d.bridgeRequestsCount.WithLabelValues(pool, status).Inc()
	return resources
}

// housekeeping listens to updates from the backend resources
func (d *TelegramDistributor) housekeeping(rStream chan *core.ResourceDiff) {
	defer d.wg.Done()
	defer close(rStream)
	defer d.ipc.StopStream()

	ticker := time.NewTimer(time.Hour * time.Duration(d.cfg.RotationPeriodHours))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.expireRequestHashKeys()
		case diff := <-rStream:
			d.oldHashring.ApplyDiff(diff)
		case <-d.shutdown:
			log.Printf("Shutting down housekeeping.")
			return
		}
	}
}

func (d *TelegramDistributor) expireRequestHashKeys() {
	keepDate := time.Now().Add(-time.Hour * time.Duration(d.cfg.RotationPeriodHours))
	for hk, t := range d.requestHashKeys {
		if t.Before(keepDate) {
			delete(d.requestHashKeys, hk)
		}
	}
}

func (d *TelegramDistributor) Init(cfg *internal.Config) {
	d.cfg = &cfg.Distributors.Telegram
	d.shutdown = make(chan bool)
	d.oldHashring = core.NewHashring()
	d.newHashring = core.NewHashring()

	d.bridgeRequestsCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "telegram_bridges_request_total",
		Help: "The total number of bridge requests",
	},
		[]string{"pool", "status"},
	)
	d.requestHashKeys = make(map[core.Hashkey]time.Time)

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

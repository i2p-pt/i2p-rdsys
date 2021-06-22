package gettor

import (
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/delivery"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/delivery/mechanisms"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/resources"
)

const (
	UpdName = "gettor"
)

type GettorUpdater struct {
	ipc delivery.Mechanism
}

func (u *GettorUpdater) Init(cfg *internal.Config) {
	u.ipc = mechanisms.NewHttpsIpc(
		"http://"+cfg.Backend.WebApi.ApiAddress+cfg.Backend.ResourcesEndpoint,
		"POST",
		cfg.Backend.ApiTokens[UpdName])
}

func (u *GettorUpdater) Shutdown() {
}

func (u *GettorUpdater) AddLinks(links []*resources.TBLink) error {
	return u.ipc.MakeJsonRequest(&links, nil)
}

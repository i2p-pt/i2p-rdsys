// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/distributors/telegram"
	tb "gopkg.in/tucnak/telebot.v2"
)

const (
	TelegramPollTimeout = 10 * time.Second
)

type TBot struct {
	bot  *tb.Bot
	dist *telegram.TelegramDistributor
}

// InitFrontend is the entry point to telegram'ss frontend.  It connects to telegram over
// the bot API and waits for user commands.
func InitFrontend(cfg *internal.Config) {
	dist := telegram.TelegramDistributor{}
	dist.Init(cfg)
	err := loadNewBridges(cfg.Distributors.Telegram.NewBridgesFile, &dist)
	if err != nil {
		log.Fatal(err)
	}

	tbot, err := newTBot(cfg.Distributors.Telegram.Token, &dist)
	if err != nil {
		log.Fatal(err)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT)
	signal.Notify(signalChan, syscall.SIGTERM)
	go func() {
		<-signalChan
		log.Printf("Caught SIGINT.")
		dist.Shutdown()

		log.Printf("Shutting down the telegram bot.")
		tbot.Stop()
	}()

	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(cfg.Distributors.Telegram.MetricsAddress, nil)

	tbot.Start()
}

func loadNewBridges(path string, dist *telegram.TelegramDistributor) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return dist.LoadNewBridges(f)
}

func newTBot(token string, dist *telegram.TelegramDistributor) (*TBot, error) {
	var t TBot
	var err error

	t.dist = dist
	t.bot, err = tb.NewBot(tb.Settings{
		Token:  token,
		Poller: &tb.LongPoller{Timeout: TelegramPollTimeout},
	})
	if err != nil {
		return nil, err
	}

	t.bot.Handle("/bridges", t.getBridges)
	return &t, nil
}

func (t *TBot) Start() {
	t.bot.Start()
}

func (t *TBot) Stop() {
	t.bot.Stop()
}

func (t *TBot) getBridges(m *tb.Message) {
	if m.Sender.IsBot {
		t.bot.Send(m.Sender, "No bridges for bots, sorry")
		return
	}

	userID := m.Sender.ID
	resources := t.dist.GetResources(userID)
	response := "Your bridges:"
	for _, r := range resources {
		response += "\n" + r.String()
	}
	t.bot.Send(m.Sender, response)
}

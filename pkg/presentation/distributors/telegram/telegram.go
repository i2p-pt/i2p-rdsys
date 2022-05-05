// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package telegram

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/persistence"
	pjson "gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/persistence/json"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/distributors/telegram"
	tb "gopkg.in/tucnak/telebot.v2"
)

const (
	TelegramPollTimeout = 10 * time.Second
)

type TBot struct {
	bot          *tb.Bot
	dist         *telegram.TelegramDistributor
	updateTokens map[string]string
}

// InitFrontend is the entry point to telegram'ss frontend.  It connects to telegram over
// the bot API and waits for user commands.
func InitFrontend(cfg *internal.Config) {
	newBridgesStore := make(map[string]persistence.Mechanism, len(cfg.Distributors.Telegram.UpdaterTokens))
	for updater := range cfg.Distributors.Telegram.UpdaterTokens {
		newBridgesStore[updater] = pjson.New(updater, cfg.Distributors.Telegram.StorageDir)
	}

	dist := telegram.TelegramDistributor{
		NewBridgesStore: newBridgesStore,
	}
	dist.Init(cfg)

	tbot, err := newTBot(cfg.Distributors.Telegram.Token, &dist)
	if err != nil {
		log.Fatal(err)
	}
	tbot.updateTokens = cfg.Distributors.Telegram.UpdaterTokens

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

	http.HandleFunc("/update", tbot.updateHandler)
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(cfg.Distributors.Telegram.ApiAddress, nil)

	tbot.Start()
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

func (t *TBot) updateHandler(w http.ResponseWriter, r *http.Request) {
	name := t.getTokenName(w, r)
	if name == "" {
		return
	}
	defer r.Body.Close()

	err := t.dist.LoadNewBridges(name, r.Body)
	if err != nil {
		log.Printf("Error loading bridges: %v", err)
		http.Error(w, "error while loading bridges", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (t *TBot) getTokenName(w http.ResponseWriter, r *http.Request) string {
	tokenLine := r.Header.Get("Authorization")
	if tokenLine == "" {
		log.Printf("Request carries no 'Authorization' HTTP header.")
		http.Error(w, "request carries no 'Authorization' HTTP header", http.StatusBadRequest)
		return ""
	}
	if !strings.HasPrefix(tokenLine, "Bearer ") {
		log.Printf("Authorization header contains no bearer token.")
		http.Error(w, "authorization header contains no bearer token", http.StatusBadRequest)
		return ""
	}
	fields := strings.Split(tokenLine, " ")
	givenToken := fields[1]

	for name, savedToken := range t.updateTokens {
		if givenToken == savedToken {
			return name
		}
	}

	log.Printf("Invalid authentication token.")
	http.Error(w, "invalid authentication token", http.StatusUnauthorized)
	return ""
}

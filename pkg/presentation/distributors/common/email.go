// Copyright (c) 2021-2022, The Tor Project, Inc.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package common

import (
	"bufio"
	"fmt"
	"log"
	"net/mail"
	"net/smtp"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap-idle"
	"github.com/emersion/go-imap/client"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/pkg/usecases/distributors"
)

const (
	durationIgnoreEmails = 24 * time.Hour
)

type SendFunction func(subject, body string) error
type IncomingEmailHandler func(msg *mail.Message, send SendFunction) error

type imapClient struct {
	*client.Client
	*idle.IdleClient
}

type emailClient struct {
	cfg             *internal.EmailConfig
	imap            *imapClient
	dist            distributors.Distributor
	incomingHandler IncomingEmailHandler
	smtpAuth        *smtp.Auth
}

func StartEmail(emailCfg *internal.EmailConfig, distCfg *internal.Config,
	dist distributors.Distributor, incomingHandler IncomingEmailHandler) {

	dist.Init(distCfg)
	smtpHost := strings.Split(emailCfg.SmtpServer, ":")[0]
	smtpAuth := smtp.PlainAuth("", emailCfg.SmtpUsername, emailCfg.SmtpPassword, smtpHost)
	imap, err := initImap(emailCfg)
	if err != nil {
		log.Fatal("Can't start the imap client: ", err)
	}

	e := emailClient{
		cfg:             emailCfg,
		imap:            imap,
		dist:            dist,
		incomingHandler: incomingHandler,
		smtpAuth:        &smtpAuth,
	}

	stop := make(chan struct{})
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT)
	signal.Notify(signalChan, syscall.SIGTERM)
	go func() {
		<-signalChan
		log.Printf("Caught SIGINT.")
		e.dist.Shutdown()

		close(stop)
		e.imap.Logout()
	}()

	if err := e.listenImapUpdates(stop); err != nil {
		log.Println("Error listening emails:", err)
	}
}

func initImap(emailCfg *internal.EmailConfig) (*imapClient, error) {
	splitedAddress := strings.Split(emailCfg.ImapServer, "://")
	if len(splitedAddress) != 2 {
		return nil, fmt.Errorf("Malformed imap server configuration: %s", emailCfg.ImapServer)
	}
	protocol := splitedAddress[0]
	serverAddress := splitedAddress[1]

	var c *client.Client
	var err error
	switch protocol {
	case "imaps":
		c, err = client.DialTLS(serverAddress, nil)
	case "imap":
		c, err = client.Dial(serverAddress)
	default:
		return nil, fmt.Errorf("Unkown protocol: %s", protocol)
	}
	if err != nil {
		return nil, err
	}

	err = c.Login(emailCfg.ImapUsername, emailCfg.ImapPassword)
	idleClient := idle.NewClient(c)
	return &imapClient{c, idleClient}, err
}

func (e *emailClient) listenImapUpdates(stop <-chan struct{}) error {
	mbox, err := e.imap.Select("INBOX", false)
	if err != nil {
		return err
	}
	e.fetchMessages(mbox)

	for {
		select {
		case <-stop:
			return nil
		default:
			update, err := e.waitForMailboxUpdate()
			if err != nil {
				log.Println("Error imap idling", err)
				continue
			}
			e.fetchMessages(update.Mailbox)
		}
	}
}

func (e *emailClient) waitForMailboxUpdate() (mboxUpdate *client.MailboxUpdate, err error) {
	// Create a channel to receive mailbox updates
	updates := make(chan client.Update, 1)
	e.imap.Updates = updates

	// Start idling
	done := make(chan error, 1)
	stop := make(chan struct{})
	go func() {
		done <- e.imap.IdleWithFallback(stop, 0)
	}()

	// Listen for updates
waitLoop:
	for {
		select {
		case update := <-updates:
			var ok bool
			mboxUpdate, ok = update.(*client.MailboxUpdate)
			if ok {
				break waitLoop
			}
		case err := <-done:
			return nil, err
		}
	}

	// We need to nil the updates channel or the client will hang on it
	// https://github.com/emersion/go-imap-idle/issues/16
	e.imap.Updates = nil
	close(stop)
	<-done

	return mboxUpdate, nil
}

func (e *emailClient) fetchMessages(mboxStatus *imap.MailboxStatus) {
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag, imap.DeletedFlag}
	seqs, err := e.imap.Search(criteria)
	if err != nil {
		log.Println("Error getting unseen messages:", err)
		return
	}

	if len(seqs) == 0 {
		return
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(seqs...)
	items := []imap.FetchItem{imap.FetchItem("BODY.PEEK[]")}

	log.Println("fetch", len(seqs), "messages from the imap server")
	messages := make(chan *imap.Message, mboxStatus.Messages)
	go func() {
		err := e.imap.Fetch(seqset, items, messages)
		if err != nil {
			log.Println("Error fetching imap messages:", err)
		}
	}()

	for msg := range messages {
		flag := ""
		for _, literal := range msg.Body {
			email, err := mail.ReadMessage(literal)
			if err != nil {
				log.Println("Error parsing incoming email", err)
				continue
			}

			send := func(subject, body string) error {
				return e.reply(email, subject, body)
			}

			err = e.incomingHandler(email, send)
			if err != nil {
				log.Println("Error handling incoming email ", email.Header.Get("Message-ID"), ":", err)

				date, err := email.Header.Date()
				if flag == "" &&
					(err != nil || date.Add(durationIgnoreEmails).Before(time.Now())) {

					log.Println("Give up with the email, marked as readed so it will not be processed anymore")
					flag = imap.SeenFlag
				}
			} else {
				// delete the email as it was fully processed
				flag = imap.DeletedFlag
			}
		}
		if flag != "" {
			seqset := new(imap.SeqSet)
			seqset.AddNum(msg.SeqNum)

			item := imap.FormatFlagsOp(imap.AddFlags, true)
			flags := []interface{}{flag}
			err := e.imap.Store(seqset, item, flags, nil)
			if err != nil {
				log.Println("Error setting the delete flag", err)
			}
		}
	}

	if err := e.imap.Expunge(nil); err != nil {
		log.Println("Error expunging messages from inbox", err)
	}
}

func (e *emailClient) reply(originalMessage *mail.Message, subject, body string) error {
	sender, err := originalMessage.Header.AddressList("From")
	if err != nil {
		return err
	}
	if len(sender) != 1 {
		return fmt.Errorf("Unexpected email from: %s", originalMessage.Header.Get("From"))
	}

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"In-Reply-To: %s\r\n"+
		"MIME-version: 1.0\r\n"+
		"Content-Type: text/plain; charset=\"utf-8\"\r\n"+
		"\r\n",
		e.cfg.Address,
		sender[0].String(),
		subject,
		originalMessage.Header.Get("Message-ID"),
	)
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		msg += scanner.Text() + "\r\n"
	}
	return smtp.SendMail(e.cfg.SmtpServer, *e.smtpAuth, e.cfg.Address, []string{sender[0].Address}, []byte(msg))
}

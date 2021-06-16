package common

import (
	"net/mail"
	"syscall"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap-idle"
	"github.com/emersion/go-imap/backend"
	"github.com/emersion/go-imap/backend/memory"
	"github.com/emersion/go-imap/server"
	"gitlab.torproject.org/tpo/anti-censorship/rdsys/internal"
)

const (
	testImapAddress = "localhost:1143"
)

var (
	testEmailCfg = internal.EmailConfig{
		Address:      "test@example.com",
		ImapServer:   "imap://" + testImapAddress,
		ImapUsername: "username",
		ImapPassword: "password",
	}
)

type testEmailDistributor struct{}

func (ted *testEmailDistributor) Init(cfg *internal.Config) {}
func (ted *testEmailDistributor) Shutdown()                 {}

func testImapServer() (*server.Server, backend.Mailbox) {
	be := memory.New()
	user, _ := be.Login(nil, testEmailCfg.ImapUsername, testEmailCfg.ImapPassword)
	mbox, _ := user.GetMailbox("INBOX")
	s := server.New(be)
	s.Addr = testImapAddress
	s.AllowInsecureAuth = true
	s.Enable(idle.NewExtension())

	go s.ListenAndServe()
	return s, mbox
}

func TestImapExistingInbox(t *testing.T) {
	s, mbox := testImapServer()
	defer s.Close()

	go timeoutDistributor(t, time.Second*5)

	handler := func(msg *mail.Message, send SendFunction) error {
		from, err := mail.ParseAddress(msg.Header.Get("From"))
		if err != nil {
			t.Fatal("Error parsing from address", err)
		}
		if from.Address != "contact@example.org" {
			t.Error("unexpected from:", from)
		}
		if msg.Header.Get("subject") != "A little message, just for you" {
			t.Error("unexpected suject:", msg.Header.Get("subject"))
		}

		go checkInboxEmptyAndExit(t, mbox)
		return nil
	}

	StartEmail(&testEmailCfg, nil, &testEmailDistributor{}, handler)
}

func timeoutDistributor(t *testing.T, duration time.Duration) {
	time.Sleep(duration)
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	t.Fatal("Timeout, no email recived")
}

func checkInboxEmptyAndExit(t *testing.T, mbox backend.Mailbox) {
	time.Sleep(time.Second)
	s, _ := mbox.Status([]imap.StatusItem{imap.StatusMessages})
	if s.Messages != 0 {
		t.Error("The message was not deleted")
	}
	syscall.Kill(syscall.Getpid(), syscall.SIGINT)
}

package infrastructure

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	mail "github.com/wneessen/go-mail"
)

type fakeSMTPClient struct {
	messages []*mail.Msg
	err      error
}

func (c *fakeSMTPClient) DialAndSendWithContext(_ context.Context, messages ...*mail.Msg) error {
	c.messages = append(c.messages, messages...)
	return c.err
}

func validEmailConfig() EmailConfig {
	return EmailConfig{Mailer: "smtp", Username: "sender@example.com", Password: "secret", Host: "smtp.example.com", Port: "465", Encryption: "ssl", From: "sender@example.com", To: "recipient@example.com"}
}

func TestEmailBuildsPlainTextJobAlert(t *testing.T) {
	fake := &fakeSMTPClient{}
	email := NewEmail(validEmailConfig())
	email.newClient = func(EmailConfig) (smtpClient, error) { return fake, nil }
	if err := email.Send(t.Context(), "Daftar Job Terbaru"); err != nil {
		t.Fatal(err)
	}
	if len(fake.messages) != 1 {
		t.Fatalf("messages=%d", len(fake.messages))
	}
	message := fake.messages[0]
	if got := message.GetFrom(); len(got) != 1 || got[0].Address != "sender@example.com" {
		t.Fatalf("from=%v", got)
	}
	if got := message.GetTo(); len(got) != 1 || got[0].Address != "recipient@example.com" {
		t.Fatalf("to=%v", got)
	}
	if got := message.GetGenHeader(mail.HeaderSubject); len(got) != 1 || got[0] != "Job Alert Harian" {
		t.Fatalf("subject=%v", got)
	}
	var rendered bytes.Buffer
	if _, err := message.WriteTo(&rendered); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), "Daftar Job Terbaru") || !strings.Contains(rendered.String(), "text/plain") {
		t.Fatalf("rendered message missing plain-text body: %s", rendered.String())
	}
}

func TestEmailRejectsInvalidConfigurationWithoutDialing(t *testing.T) {
	tests := []EmailConfig{
		{},
		func() EmailConfig { config := validEmailConfig(); config.Mailer = "sendmail"; return config }(),
		func() EmailConfig { config := validEmailConfig(); config.Encryption = "tls"; return config }(),
		func() EmailConfig { config := validEmailConfig(); config.Port = "invalid"; return config }(),
	}
	for _, config := range tests {
		email := NewEmail(config)
		email.newClient = func(EmailConfig) (smtpClient, error) {
			t.Fatal("SMTP client must not be created for invalid configuration")
			return nil, nil
		}
		if err := email.Send(t.Context(), "jobs"); err == nil {
			t.Fatalf("expected config %#v to fail", config)
		}
	}
}

func TestEmailAttemptsSendOnce(t *testing.T) {
	fake := &fakeSMTPClient{err: errors.New("ambiguous SMTP result")}
	email := NewEmail(validEmailConfig())
	email.newClient = func(EmailConfig) (smtpClient, error) { return fake, nil }
	if err := email.Send(t.Context(), "jobs"); err == nil || len(fake.messages) != 1 {
		t.Fatalf("err=%v messages=%d", err, len(fake.messages))
	}
}

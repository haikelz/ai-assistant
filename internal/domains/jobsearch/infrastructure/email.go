package infrastructure

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	mail "github.com/wneessen/go-mail"
)

type EmailConfig struct {
	Mailer, Username, Password, Host, Port, Encryption, From, To string
}

type smtpClient interface {
	DialAndSendWithContext(context.Context, ...*mail.Msg) error
}

type Email struct {
	config    EmailConfig
	newClient func(EmailConfig) (smtpClient, error)
}

func NewEmail(config EmailConfig) *Email {
	return &Email{config: config, newClient: newSMTPClient}
}

func (e *Email) Send(ctx context.Context, body string) error {
	if err := validateEmailConfig(e.config); err != nil {
		return err
	}
	message := mail.NewMsg()
	if err := message.From(e.config.From); err != nil {
		return fmt.Errorf("configure email sender: %w", err)
	}
	if err := message.To(e.config.To); err != nil {
		return fmt.Errorf("configure email recipient: %w", err)
	}
	message.Subject("Job Alert Harian")
	message.SetBodyString(mail.TypeTextPlain, body)
	client, err := e.newClient(e.config)
	if err != nil {
		return fmt.Errorf("configure SMTP client: %w", err)
	}
	if err := client.DialAndSendWithContext(ctx, message); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}

func validateEmailConfig(config EmailConfig) error {
	values := map[string]string{
		"MAIL_MAILER": config.Mailer, "MAIL_USERNAME": config.Username,
		"MAIL_PASSWORD": config.Password, "MAIL_HOST": config.Host,
		"MAIL_PORT": config.Port, "MAIL_ENCRYPTION": config.Encryption,
		"MAIL_FROM": config.From, "MAIL_TO": config.To,
	}
	for name, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required when scheduled email is enabled", name)
		}
	}
	if !strings.EqualFold(config.Mailer, "smtp") {
		return fmt.Errorf("MAIL_MAILER must be smtp")
	}
	if !strings.EqualFold(config.Encryption, "ssl") {
		return fmt.Errorf("MAIL_ENCRYPTION must be ssl")
	}
	port, err := strconv.Atoi(config.Port)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("MAIL_PORT must be a number between 1 and 65535")
	}
	return nil
}

func newSMTPClient(config EmailConfig) (smtpClient, error) {
	port, _ := strconv.Atoi(config.Port)
	return mail.NewClient(
		config.Host,
		mail.WithPort(port),
		mail.WithSSL(),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(config.Username),
		mail.WithPassword(config.Password),
	)
}

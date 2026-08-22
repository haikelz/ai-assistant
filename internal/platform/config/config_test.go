package config

import "testing"

func TestLoadReadsSMTPEmailConfiguration(t *testing.T) {
	values := map[string]string{
		"MAIL_MAILER": "smtp", "MAIL_USERNAME": "user", "MAIL_PASSWORD": "password",
		"MAIL_HOST": "smtp.example.com", "MAIL_PORT": "465", "MAIL_ENCRYPTION": "ssl",
		"MAIL_FROM": "sender@example.com", "MAIL_TO": "recipient@example.com",
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
	config := Load()
	actual := map[string]string{
		"MAIL_MAILER": config.MailMailer, "MAIL_USERNAME": config.MailUsername,
		"MAIL_PASSWORD": config.MailPassword, "MAIL_HOST": config.MailHost,
		"MAIL_PORT": config.MailPort, "MAIL_ENCRYPTION": config.MailEncryption,
		"MAIL_FROM": config.MailFrom, "MAIL_TO": config.MailTo,
	}
	for name, expected := range values {
		if actual[name] != expected {
			t.Errorf("%s=%q, expected %q", name, actual[name], expected)
		}
	}
}

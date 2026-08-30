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

func TestLoadReadsCuratedJobAlertConfiguration(t *testing.T) {
	values := map[string]string{
		"JOB_ALERT_DB_PATH": "/tmp/jobs.db", "JOB_ALERT_PIPELINE_ENABLED": "false",
		"GLINTS_ENABLED": "false", "LINKEDIN_ENABLED": "true", "JOB_ALERT_MAX_QUERIES": "3",
		"JOB_ALERT_AI_BATCH_SIZE": "8", "JOB_ALERT_MIN_MATCH_SCORE": "75.5",
		"LINKEDIN_PAGES": "2", "LINKEDIN_MAX_DETAILS": "4", "LINKEDIN_POSTED_WITHIN_HOURS": "72", "LINKEDIN_DISTANCE": "50",
		"LINKEDIN_JOB_TYPES": "F, C", "LINKEDIN_COMPANY_IDS": "101,202",
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
	config := Load()
	if config.JobAlertDBPath != "/tmp/jobs.db" || config.JobAlertPipelineEnabled || config.GlintsEnabled || !config.LinkedInEnabled || config.JobAlertMaxQueries != 3 || config.JobAlertBatchSize != 8 || config.JobAlertMinMatchScore != 75.5 || config.LinkedInPages != 2 || config.LinkedInMaxDetails != 4 || config.LinkedInPostedWithinHours != 72 || config.LinkedInDistance != 50 || len(config.LinkedInJobTypes) != 2 || len(config.LinkedInCompanyIDs) != 2 {
		t.Fatalf("curated job-alert config=%#v", config)
	}
}

func TestLoadDisablesLinkedInByDefault(t *testing.T) {
	t.Setenv("LINKEDIN_ENABLED", "")
	if config := Load(); config.LinkedInEnabled {
		t.Fatal("LinkedIn must be opt-in")
	}
}

func TestLoadDefaultsJobAlertMatchScoreToOne(t *testing.T) {
	t.Setenv("JOB_ALERT_MIN_MATCH_SCORE", "")
	if config := Load(); config.JobAlertMinMatchScore != 1 {
		t.Fatalf("JOB_ALERT_MIN_MATCH_SCORE=%v, expected 1", config.JobAlertMinMatchScore)
	}
}

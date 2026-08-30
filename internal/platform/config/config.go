package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	FinanceAddress, JobSearchAddress, DatabasePath, JobAlertConfigPath string
	JobAlertDBPath                                                     string
	JobAlertPipelineEnabled, GlintsEnabled, LinkedInEnabled            bool
	JobAlertMaxQueries, JobAlertBatchSize                              int
	JobAlertMinMatchScore                                              float64
	LinkedInPages, LinkedInMaxDetails                                  int
	LinkedInPostedWithinHours, LinkedInDistance                        int
	LinkedInJobTypes, LinkedInCompanyIDs                               []string
	WhatsAppSessionPath, WhatsAppGatewayURL                            string
	SumopodResponsesURL                                                string
	AIProvider, AIModel                                                string
	SumopodAPIKey, OpenAIAPIKey, GoogleAPIKey                          string
	OpenAIResponsesURL, GoogleGenerativeURL                            string
	TelegramBotToken, TelegramUserID, WhatsAppRecipient                string
	MailMailer, MailUsername, MailPassword, MailHost                   string
	MailPort, MailEncryption, MailFrom, MailTo                         string
	SpreadsheetID, ServiceAccountBase64                                string
}

func Load() Config {
	return Config{
		FinanceAddress: getenv("FINANCE_ADDR", "127.0.0.1:8080"), JobSearchAddress: getenv("LOKER_ADDR", "127.0.0.1:8081"), DatabasePath: getenv("FINANCE_DB_PATH", "/root/.picoclaw/finance.db"), JobAlertConfigPath: getenv("JOB_ALERT_CONFIG_PATH", "/root/.picoclaw/job-alert.json"),
		JobAlertDBPath: getenv("JOB_ALERT_DB_PATH", "/root/.picoclaw/jobs.db"), JobAlertPipelineEnabled: getenvBool("JOB_ALERT_PIPELINE_ENABLED", true), GlintsEnabled: getenvBool("GLINTS_ENABLED", true), LinkedInEnabled: getenvBool("LINKEDIN_ENABLED", false), JobAlertMaxQueries: getenvInt("JOB_ALERT_MAX_QUERIES", 5), JobAlertBatchSize: getenvInt("JOB_ALERT_AI_BATCH_SIZE", 5), JobAlertMinMatchScore: getenvFloat("JOB_ALERT_MIN_MATCH_SCORE", 1),
		LinkedInPages: getenvInt("LINKEDIN_PAGES", 2), LinkedInMaxDetails: getenvInt("LINKEDIN_MAX_DETAILS", 3), LinkedInPostedWithinHours: getenvInt("LINKEDIN_POSTED_WITHIN_HOURS", 168), LinkedInDistance: getenvInt("LINKEDIN_DISTANCE", 25), LinkedInJobTypes: getenvCSV("LINKEDIN_JOB_TYPES"), LinkedInCompanyIDs: getenvCSV("LINKEDIN_COMPANY_IDS"),
		WhatsAppSessionPath: getenv("WHATSAPP_SESSION_PATH", "/root/.picoclaw/whatsapp.db"), WhatsAppGatewayURL: getenv("WHATSAPP_GATEWAY_URL", "http://127.0.0.1:8081/internal/whatsapp/send"),
		SumopodResponsesURL: getenv("SUMOPOD_RESPONSES_URL", "https://ai.sumopod.com/v1/responses"), AIProvider: getenv("AI_PROVIDER", "sumopod"), AIModel: strings.TrimSpace(os.Getenv("AI_MODEL")),
		SumopodAPIKey: strings.TrimSpace(os.Getenv("SUMOPOD_API_KEY")), OpenAIAPIKey: strings.TrimSpace(os.Getenv("OPENAI_API_KEY")), GoogleAPIKey: strings.TrimSpace(os.Getenv("GOOGLE_API_KEY")),
		OpenAIResponsesURL: getenv("OPENAI_RESPONSES_URL", "https://api.openai.com/v1/responses"), GoogleGenerativeURL: getenv("GOOGLE_GENERATIVE_URL", "https://generativelanguage.googleapis.com/v1beta"),
		TelegramBotToken: strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")), TelegramUserID: strings.TrimSpace(os.Getenv("TELEGRAM_USER_ID")), WhatsAppRecipient: strings.TrimSpace(os.Getenv("WHATSAPP_RECIPIENT")), SpreadsheetID: strings.TrimSpace(os.Getenv("GOOGLE_SHEETS_SPREADSHEET_ID")), ServiceAccountBase64: strings.TrimSpace(os.Getenv("GOOGLE_SERVICE_ACCOUNT_JSON_BASE64")),
		MailMailer: strings.TrimSpace(os.Getenv("MAIL_MAILER")), MailUsername: strings.TrimSpace(os.Getenv("MAIL_USERNAME")), MailPassword: strings.TrimSpace(os.Getenv("MAIL_PASSWORD")), MailHost: strings.TrimSpace(os.Getenv("MAIL_HOST")), MailPort: strings.TrimSpace(os.Getenv("MAIL_PORT")), MailEncryption: strings.TrimSpace(os.Getenv("MAIL_ENCRYPTION")), MailFrom: strings.TrimSpace(os.Getenv("MAIL_FROM")), MailTo: strings.TrimSpace(os.Getenv("MAIL_TO")),
	}
}

func getenvCSV(key string) []string {
	var values []string
	for _, value := range strings.Split(os.Getenv(key), ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func getenvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvInt(key string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvFloat(key string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(key)), 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

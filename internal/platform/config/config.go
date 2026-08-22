package config

import (
	"os"
	"strings"
)

type Config struct {
	FinanceAddress, JobSearchAddress, DatabasePath, JobAlertConfigPath string
	WhatsAppSessionPath, WhatsAppGatewayURL                            string
	SumopodResponsesURL                                                string
	AIProvider, AIModel                                                string
	SumopodAPIKey, OpenAIAPIKey, GoogleAPIKey                          string
	OpenAIResponsesURL, GoogleGenerativeURL                            string
	TelegramBotToken, TelegramUserID, WhatsAppRecipient                string
	SpreadsheetID, ServiceAccountBase64                                string
}

func Load() Config {
	return Config{
		FinanceAddress: getenv("FINANCE_ADDR", "127.0.0.1:8080"), JobSearchAddress: getenv("LOKER_ADDR", "127.0.0.1:8081"), DatabasePath: getenv("FINANCE_DB_PATH", "/root/.picoclaw/finance.db"), JobAlertConfigPath: getenv("JOB_ALERT_CONFIG_PATH", "/root/.picoclaw/job-alert.json"),
		WhatsAppSessionPath: getenv("WHATSAPP_SESSION_PATH", "/root/.picoclaw/whatsapp.db"), WhatsAppGatewayURL: getenv("WHATSAPP_GATEWAY_URL", "http://127.0.0.1:8081/internal/whatsapp/send"),
		SumopodResponsesURL: getenv("SUMOPOD_RESPONSES_URL", "https://ai.sumopod.com/v1/responses"), AIProvider: getenv("AI_PROVIDER", "sumopod"), AIModel: strings.TrimSpace(os.Getenv("AI_MODEL")),
		SumopodAPIKey: strings.TrimSpace(os.Getenv("SUMOPOD_API_KEY")), OpenAIAPIKey: strings.TrimSpace(os.Getenv("OPENAI_API_KEY")), GoogleAPIKey: strings.TrimSpace(os.Getenv("GOOGLE_API_KEY")),
		OpenAIResponsesURL: getenv("OPENAI_RESPONSES_URL", "https://api.openai.com/v1/responses"), GoogleGenerativeURL: getenv("GOOGLE_GENERATIVE_URL", "https://generativelanguage.googleapis.com/v1beta"),
		TelegramBotToken: strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")), TelegramUserID: strings.TrimSpace(os.Getenv("TELEGRAM_USER_ID")), WhatsAppRecipient: strings.TrimSpace(os.Getenv("WHATSAPP_RECIPIENT")), SpreadsheetID: strings.TrimSpace(os.Getenv("GOOGLE_SHEETS_SPREADSHEET_ID")), ServiceAccountBase64: strings.TrimSpace(os.Getenv("GOOGLE_SERVICE_ACCOUNT_JSON_BASE64")),
	}
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

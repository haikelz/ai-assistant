package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type Telegram struct {
	client              *http.Client
	token, userID, base string
}

func NewTelegram(client *http.Client, token, userID, baseURL string) *Telegram {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	return &Telegram{client, token, userID, strings.TrimRight(baseURL, "/")}
}

func (t *Telegram) Send(ctx context.Context, message string) error {
	if strings.TrimSpace(t.token) == "" || strings.TrimSpace(t.userID) == "" {
		_, stderrErr := fmt.Fprintln(os.Stderr, "job-alert: Telegram credentials missing, printing to stdout")
		_, stdoutErr := fmt.Fprintln(os.Stdout, message)
		return errors.Join(stderrErr, stdoutErr)
	}
	for _, chunk := range domain.SplitTelegramMessage(message, 4000) {
		if err := t.sendChunk(ctx, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (t *Telegram) sendChunk(ctx context.Context, chunk string) error {
	body, err := json.Marshal(map[string]string{
		"chat_id":                  t.userID,
		"text":                     chunk,
		"parse_mode":               "Markdown",
		"disable_web_page_preview": "true",
	})
	if err != nil {
		return fmt.Errorf("encode Telegram message: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/bot%s/sendMessage", t.base, t.token), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("send Telegram message: %w", err)
	}
	responseBody, readErr := readAndCloseResponse(resp, 64<<10)
	if resp.StatusCode != http.StatusOK {
		statusErr := fmt.Errorf("Telegram status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
		return errors.Join(statusErr, readErr)
	}
	return readErr
}

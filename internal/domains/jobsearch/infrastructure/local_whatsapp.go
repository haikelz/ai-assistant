package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type LocalWhatsApp struct {
	client *http.Client
	url    string
}

func NewLocalWhatsApp(client *http.Client, url string) *LocalWhatsApp {
	if client == nil {
		client = http.DefaultClient
	}
	return &LocalWhatsApp{client: client, url: strings.TrimSpace(url)}
}

func (w *LocalWhatsApp) Send(ctx context.Context, message string) error {
	body, err := json.Marshal(map[string]string{"message": message})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := w.client.Do(request)
	if err != nil {
		return fmt.Errorf("send local WhatsApp message: %w", err)
	}
	responseBody, readErr := readAndCloseResponse(response, 4<<10)
	if response.StatusCode != http.StatusNoContent {
		return errors.Join(fmt.Errorf("local WhatsApp status %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody))), readErr)
	}
	return readErr
}

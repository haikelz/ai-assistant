package infrastructure

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ai-assistant/internal/domains/ai/domain"
)

const MaxResponseBytes = 8 << 20

type SumopodProxy struct {
	client   *http.Client
	endpoint string
}

func NewSumopodProxy(client *http.Client, endpoint string) *SumopodProxy {
	if client == nil {
		client = http.DefaultClient
	}
	return &SumopodProxy{client: client, endpoint: endpoint}
}

func (p *SumopodProxy) Forward(ctx context.Context, authorization string, body []byte) (domain.ProxyResponse, error) {
	prepared, isGPT, err := PrepareResponsesBody(body)
	if err != nil {
		return domain.ProxyResponse{}, fmt.Errorf("%w: %v", domain.ErrInvalidRequest, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(prepared))
	if err != nil {
		return domain.ProxyResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return domain.ProxyResponse{}, err
	}
	defer response.Body.Close()
	contentType := response.Header.Get("Content-Type")
	if isGPT && strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		completed, err := ExtractCompletedResponse(response.Body)
		return domain.ProxyResponse{Status: response.StatusCode, ContentType: "application/json", Body: completed}, err
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxResponseBytes+1))
	if err != nil {
		return domain.ProxyResponse{}, err
	}
	if len(data) > MaxResponseBytes {
		return domain.ProxyResponse{}, fmt.Errorf("upstream response exceeds %d bytes", MaxResponseBytes)
	}
	return domain.ProxyResponse{Status: response.StatusCode, ContentType: contentType, Body: data}, nil
}

func PrepareResponsesBody(body []byte) ([]byte, bool, error) {
	var metadata struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, false, fmt.Errorf("decode request metadata: %w", err)
	}
	if !strings.Contains(strings.ToLower(metadata.Model), "gpt") {
		return body, false, nil
	}
	var request map[string]json.RawMessage
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, true, fmt.Errorf("decode GPT request: %w", err)
	}
	delete(request, "temperature")
	prepared, err := json.Marshal(request)
	return prepared, true, err
}

func ExtractCompletedResponse(body io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64<<10), MaxResponseBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event struct {
			Type     string          `json:"type"`
			Response json.RawMessage `json:"response"`
		}
		if json.Unmarshal([]byte(data), &event) == nil && event.Type == "response.completed" && len(event.Response) > 0 {
			return event.Response, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("response.completed event not found")
}

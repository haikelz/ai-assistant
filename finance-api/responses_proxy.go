package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (s *server) handleResponsesProxy(w http.ResponseWriter, r *http.Request) {
	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	defer body.Close()
	requestBody, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, "could not read Sumopod request", http.StatusBadRequest)
		return
	}
	requestBody, isGPT, err := prepareSumopodResponsesBody(requestBody)
	if err != nil {
		http.Error(w, "could not prepare Sumopod request", http.StatusBadRequest)
		return
	}

	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.sumopodResponsesURL, bytes.NewReader(requestBody))
	if err != nil {
		http.Error(w, "could not create Sumopod request", http.StatusInternalServerError)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	if authorization := r.Header.Get("Authorization"); authorization != "" {
		request.Header.Set("Authorization", authorization)
	}

	response, err := s.httpClient.Do(request)
	if err != nil {
		http.Error(w, "Sumopod request failed", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	if isGPT && strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		completed, err := extractCompletedResponsesEvent(response.Body)
		if err != nil {
			http.Error(w, "invalid Sumopod GPT event stream", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(completed)
		return
	}
	w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	w.WriteHeader(response.StatusCode)
	if _, err := io.Copy(w, response.Body); err != nil {
		return
	}
}

func prepareSumopodResponsesBody(body []byte) ([]byte, bool, error) {
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
	if err != nil {
		return nil, true, fmt.Errorf("encode GPT request: %w", err)
	}
	return prepared, true, nil
}

func extractCompletedResponsesEvent(body io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), maxRequestBodyBytes)
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
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if event.Type == "response.completed" && len(event.Response) > 0 {
			return event.Response, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("response.completed event not found")
}

package infrastructure

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func responsesText(resp *http.Response) (string, error) {
	type response struct {
		Output []struct{ Content []struct{ Text string } }
	}
	var r response
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		s := bufio.NewScanner(resp.Body)
		s.Buffer(make([]byte, 64<<10), 8<<20)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var e struct {
				Type     string
				Response response
			}
			if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &e) == nil && e.Type == "response.completed" {
				r = e.Response
				break
			}
		}
		if err := s.Err(); err != nil {
			return "", err
		}
	} else if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	for _, o := range r.Output {
		for _, c := range o.Content {
			if c.Text != "" {
				return c.Text, nil
			}
		}
	}
	return "", fmt.Errorf("assessment response text missing")
}

func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```json"); i >= 0 {
		s = s[i+7:]
	} else if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func providerStatusError(provider string, response *http.Response) error {
	body, readErr := readAndCloseResponse(response, 4<<10)
	statusErr := fmt.Errorf("%s provider returned status %d", provider, response.StatusCode)
	if detail := strings.TrimSpace(string(body)); detail != "" {
		statusErr = fmt.Errorf("%w: %s", statusErr, detail)
	}
	return errors.Join(statusErr, readErr)
}

func readAndCloseResponse(response *http.Response, limit int64) ([]byte, error) {
	body, readErr := io.ReadAll(io.LimitReader(response.Body, limit))
	closeErr := closeResponseBody(response)
	if readErr != nil {
		readErr = fmt.Errorf("read response body: %w", readErr)
	}
	return body, errors.Join(readErr, closeErr)
}

func closeResponseBody(response *http.Response) error {
	if err := response.Body.Close(); err != nil {
		return fmt.Errorf("close response body: %w", err)
	}
	return nil
}

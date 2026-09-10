package providers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func (l *LinkedIn) get(ctx context.Context, endpoint string, limit int64) ([]byte, error) {
	if err := l.wait(ctx); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create LinkedIn request: %w", err)
	}
	request.Header.Set("User-Agent", linkedInUserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := l.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send LinkedIn request: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, limit))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(wrapLinkedInError("read response", readErr), wrapLinkedInError("close response", closeErr))
	}
	if isLinkedInBlockedStatus(response.StatusCode) {
		l.markBlocked()
		return nil, fmt.Errorf("%w: LinkedIn status %d", domain.ErrProviderBlocked, response.StatusCode)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LinkedIn status %d", response.StatusCode)
	}
	if isLinkedInChallenge(body) {
		l.markBlocked()
		return nil, fmt.Errorf("%w: LinkedIn challenge page", domain.ErrProviderBlocked)
	}
	return body, nil
}

func (l *LinkedIn) wait(ctx context.Context) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.blocked {
		return domain.ErrProviderBlocked
	}
	wait := l.config.MinInterval - time.Since(l.lastRequest)
	if !l.lastRequest.IsZero() && wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	l.lastRequest = time.Now()
	return nil
}

func (l *LinkedIn) markBlocked() {
	l.mutex.Lock()
	l.blocked = true
	l.mutex.Unlock()
}

func isLinkedInBlockedStatus(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests || status == 999
}

func isLinkedInChallenge(body []byte) bool {
	value := strings.ToLower(string(body))
	return strings.Contains(value, "captcha") || strings.Contains(value, "challenge") || strings.Contains(value, "verify you are human") || strings.Contains(value, "security verification")
}

func wrapLinkedInError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

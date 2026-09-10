package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

type config struct {
	URL                string
	Username           string
	Password           string
	ChromiumPath       string
	UsernameSelector   string
	PasswordSelector   string
	LoginSelector      string
	AttendanceURL      string
	AttendanceSelector string
	ClockInSelector    string
	ClockOutSelector   string
	Timeout            time.Duration
	GeoLatitude        *float64
	GeoLongitude       *float64
}

type cdpResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type cdpClient struct {
	conn    *websocket.Conn
	nextID  atomic.Int64
	mu      sync.Mutex
	pending map[int64]chan cdpResponse
}

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "clock-in" && os.Args[1] != "clock-out") {
		fmt.Fprintln(os.Stderr, "usage: attendance <clock-in|clock-out>")
		os.Exit(2)
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "attendance: configuration error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	if err := run(ctx, cfg, os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "attendance: %s failed: %v\n", os.Args[1], err)
		os.Exit(1)
	}
	fmt.Printf("attendance: %s completed successfully\n", os.Args[1])
}

func loadConfig() (config, error) {
	cfg := config{
		URL:                envOr("STARCO_URL", ""),
		Username:           strings.TrimSpace(os.Getenv("STARCO_USERNAME")),
		Password:           os.Getenv("STARCO_PASSWORD"),
		ChromiumPath:       envOr("STARCO_CHROMIUM_PATH", "/usr/bin/chromium-browser"),
		UsernameSelector:   strings.TrimSpace(os.Getenv("STARCO_USERNAME_SELECTOR")),
		PasswordSelector:   strings.TrimSpace(os.Getenv("STARCO_PASSWORD_SELECTOR")),
		LoginSelector:      strings.TrimSpace(os.Getenv("STARCO_LOGIN_SELECTOR")),
		AttendanceURL:      strings.TrimSpace(os.Getenv("STARCO_ATTENDANCE_URL")),
		AttendanceSelector: strings.TrimSpace(os.Getenv("STARCO_ATTENDANCE_SELECTOR")),
		ClockInSelector:    envOr("STARCO_CLOCK_IN_SELECTOR", "#clock-in"),
		ClockOutSelector:   envOr("STARCO_CLOCK_OUT_SELECTOR", "#clock-out"),
		Timeout:            90 * time.Second,
	}
	if raw := strings.TrimSpace(os.Getenv("STARCO_TIMEOUT_SECONDS")); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 10 || seconds > 300 {
			return config{}, errors.New("STARCO_TIMEOUT_SECONDS must be between 10 and 300")
		}
		cfg.Timeout = time.Duration(seconds) * time.Second
	}
	geoLatitude, hasLatitude, err := optionalFloat("STARCO_GEO_LATITUDE")
	if err != nil {
		return config{}, err
	}
	geoLongitude, hasLongitude, err := optionalFloat("STARCO_GEO_LONGITUDE")
	if err != nil {
		return config{}, err
	}
	if hasLatitude != hasLongitude {
		return config{}, errors.New("STARCO_GEO_LATITUDE and STARCO_GEO_LONGITUDE must be set together")
	}
	cfg.GeoLatitude, cfg.GeoLongitude = geoLatitude, geoLongitude
	if cfg.Username == "" {
		return config{}, errors.New("STARCO_USERNAME is required")
	}
	if cfg.Password == "" {
		return config{}, errors.New("STARCO_PASSWORD is required")
	}
	if cfg.URL == "" {
		return config{}, errors.New("STARCO_URL is required")
	}
	if _, err := os.Stat(cfg.ChromiumPath); err != nil {
		fallback, lookupErr := chromiumFallback(cfg.ChromiumPath)
		if lookupErr != nil {
			return config{}, fmt.Errorf("chromium executable not found at %s", cfg.ChromiumPath)
		}
		cfg.ChromiumPath = fallback
	}
	return cfg, nil
}

func optionalFloat(key string) (*float64, bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil, false, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, false, fmt.Errorf("%s must be a number", key)
	}
	return &value, true, nil
}

func chromiumFallback(primary string) (string, error) {
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"} {
		if path, err := exec.LookPath(name); err == nil && path != primary {
			return path, nil
		}
	}
	return "", errors.New("no chromium executable in PATH")
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func run(ctx context.Context, cfg config, action string) error {
	port, err := freePort()
	if err != nil {
		return err
	}
	profileDir, err := os.MkdirTemp("", "starco-chromium-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(profileDir)

	cmd := exec.CommandContext(ctx, cfg.ChromiumPath,
		"--headless", "--no-sandbox", "--disable-dev-shm-usage", "--disable-gpu",
		"--no-first-run", "--no-default-browser-check",
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1", "--user-data-dir="+profileDir, "about:blank",
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start chromium: %w", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	wsURL, err := waitForWebSocket(ctx, port)
	if err != nil {
		return err
	}
	client, err := newCDPClient(ctx, wsURL)
	if err != nil {
		return err
	}
	defer client.Close()
	_ = client.call(ctx, "Page.enable", nil, nil)
	_ = client.call(ctx, "Runtime.enable", nil, nil)
	if cfg.GeoLatitude != nil && cfg.GeoLongitude != nil {
		// Starco records clock-in coordinates; headless Chromium denies the
		// geolocation prompt, so supply a position before any page loads.
		override := map[string]any{"latitude": *cfg.GeoLatitude, "longitude": *cfg.GeoLongitude, "accuracy": 50.0}
		if err := client.call(ctx, "Emulation.setGeolocationOverride", override, nil); err != nil {
			return fmt.Errorf("apply geolocation override: %w", err)
		}
	}

	if err := navigate(ctx, client, cfg.URL); err != nil {
		return fmt.Errorf("open Starco: %w", err)
	}

	usernameSelector := cfg.UsernameSelector
	if usernameSelector == "" {
		usernameSelector = `input[name="username"],input[name="email"],input[type="email"],input[autocomplete="username"],input[type="text"]`
	}
	passwordSelector := cfg.PasswordSelector
	if passwordSelector == "" {
		passwordSelector = `input[type="password"],input[name="password"],input[autocomplete="current-password"]`
	}
	if err := setInput(ctx, client, usernameSelector, cfg.Username); err != nil {
		return fmt.Errorf("fill username: %w", err)
	}
	if err := setInput(ctx, client, passwordSelector, cfg.Password); err != nil {
		return fmt.Errorf("fill password: %w", err)
	}
	if cfg.LoginSelector != "" {
		err = clickSelector(ctx, client, cfg.LoginSelector)
	} else {
		err = clickByText(ctx, client, []string{"login", "log in", "sign in", "masuk"}, true)
	}
	if err != nil {
		return fmt.Errorf("click login: %w", err)
	}
	if err := waitUntil(ctx, 500*time.Millisecond, func() (bool, error) {
		return evaluateBool(ctx, client, fmt.Sprintf(`document.querySelector(%s) === null`, jsString(passwordSelector)))
	}); err != nil {
		return errors.New("login did not complete; configure STARCO_*_SELECTOR overrides if Starco changed")
	}

	if cfg.AttendanceURL != "" {
		if err := navigate(ctx, client, cfg.AttendanceURL); err != nil {
			return err
		}
	} else if cfg.AttendanceSelector != "" {
		if err := clickSelector(ctx, client, cfg.AttendanceSelector); err != nil {
			return err
		}
		time.Sleep(1500 * time.Millisecond)
	} else {
		_ = clickByText(ctx, client, []string{"attendance", "absensi", "presensi", "kehadiran"}, false)
		time.Sleep(1500 * time.Millisecond)
	}

	selector := cfg.ClockInSelector
	labels := []string{"clock in", "check in", "absen masuk", "masuk kerja", "hadir"}
	if action == "clock-out" {
		selector = cfg.ClockOutSelector
		labels = []string{"clock out", "check out", "absen pulang", "pulang kerja", "pulang"}
	}
	err = clickSelector(ctx, client, selector)
	if err != nil {
		// Starco only injects #clock-out after a clock-in, so fall back to
		// matching the visible button text when the stable id is absent.
		err = clickByText(ctx, client, labels, true)
	}
	if err != nil {
		return fmt.Errorf("click %s: %w", action, err)
	}

	// One submission only. The scheduler records success and will not submit the
	// same action again on the same Jakarta date.
	time.Sleep(2500 * time.Millisecond)
	return nil
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitForWebSocket(ctx context.Context, port int) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/json", port)
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		resp, err := client.Get(url)
		if err == nil {
			var tabs []struct {
				Type                 string `json:"type"`
				WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&tabs)
			_ = resp.Body.Close()
			if decodeErr == nil {
				for _, tab := range tabs {
					if tab.Type == "page" && tab.WebSocketDebuggerURL != "" {
						return tab.WebSocketDebuggerURL, nil
					}
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func newCDPClient(ctx context.Context, wsURL string) (*cdpClient, error) {
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	client := &cdpClient{conn: conn, pending: make(map[int64]chan cdpResponse)}
	go client.readLoop()
	return client, nil
}

func (c *cdpClient) Close() { _ = c.conn.Close(websocket.StatusNormalClosure, "done") }

func (c *cdpClient) readLoop() {
	for {
		_, data, err := c.conn.Read(context.Background())
		if err != nil {
			return
		}
		var response cdpResponse
		if json.Unmarshal(data, &response) != nil || response.ID == 0 {
			continue
		}
		c.mu.Lock()
		ch := c.pending[response.ID]
		delete(c.pending, response.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- response
		}
	}
}

func (c *cdpClient) call(ctx context.Context, method string, params any, out any) error {
	id := c.nextID.Add(1)
	ch := make(chan cdpResponse, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	message := map[string]any{"id": id, "method": method}
	if params != nil {
		message["params"] = params
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if err := c.conn.Write(ctx, websocket.MessageText, payload); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case response := <-ch:
		if response.Error != nil {
			return fmt.Errorf("CDP %s: %s", method, response.Error.Message)
		}
		if out != nil && len(response.Result) > 0 {
			return json.Unmarshal(response.Result, out)
		}
		return nil
	}
}

func navigate(ctx context.Context, client *cdpClient, url string) error {
	if err := client.call(ctx, "Page.navigate", map[string]any{"url": url}, nil); err != nil {
		return err
	}
	return waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		return evaluateBool(ctx, client, `document.readyState === "complete" || document.readyState === "interactive"`)
	})
}

func setInput(ctx context.Context, client *cdpClient, selector, value string) error {
	expression := fmt.Sprintf(`(() => {
const el=document.querySelector(%s); if(!el) return false;
const setter=Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,"value").set;
setter.call(el,%s); el.dispatchEvent(new Event("input",{bubbles:true}));
el.dispatchEvent(new Event("change",{bubbles:true})); return true; })()`, jsString(selector), jsString(value))
	ok, err := evaluateBool(ctx, client, expression)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("element not found for selector %q", selector)
	}
	return nil
}

func clickSelector(ctx context.Context, client *cdpClient, selector string) error {
	ok, err := evaluateBool(ctx, client, fmt.Sprintf(`(() => { const el=document.querySelector(%s); if(!el||el.disabled)return false; el.click(); return true; })()`, jsString(selector)))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("clickable element not found for selector %q", selector)
	}
	return nil
}

func clickByText(ctx context.Context, client *cdpClient, labels []string, required bool) error {
	labelsJSON, _ := json.Marshal(labels)
	expression := fmt.Sprintf(`(() => {
const labels=%s.map(v=>v.toLowerCase());
const candidates=Array.from(document.querySelectorAll('button,a,[role="button"],input[type="submit"]'));
const visible=el=>!!(el.offsetWidth||el.offsetHeight||el.getClientRects().length);
for(const el of candidates){const text=((el.innerText||el.value||el.getAttribute('aria-label')||'')+'').trim().toLowerCase();
if(!visible(el)||el.disabled)continue; if(labels.some(label=>text===label||text.includes(label))){el.click();return true;}}
return false;})()`, string(labelsJSON))
	ok, err := evaluateBool(ctx, client, expression)
	if err != nil {
		return err
	}
	if !ok && required {
		return fmt.Errorf("no visible clickable element matched %s", strings.Join(labels, ", "))
	}
	return nil
}

func evaluateBool(ctx context.Context, client *cdpClient, expression string) (bool, error) {
	var result struct {
		Result struct {
			Type  string `json:"type"`
			Value bool   `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := client.call(ctx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true}, &result); err != nil {
		return false, err
	}
	if len(result.ExceptionDetails) > 0 && string(result.ExceptionDetails) != "null" {
		return false, errors.New("javascript evaluation failed")
	}
	if result.Result.Type != "boolean" {
		return false, fmt.Errorf("javascript returned %s instead of boolean", result.Result.Type)
	}
	return result.Result.Value, nil
}

func waitUntil(ctx context.Context, interval time.Duration, fn func() (bool, error)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		ok, err := fn()
		if err == nil && ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func jsString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
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

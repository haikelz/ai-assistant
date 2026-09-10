package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	chromium := filepath.Join(t.TempDir(), "chromium")
	if err := os.WriteFile(chromium, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	emptyPath := t.TempDir()

	tests := map[string]struct {
		env     map[string]string
		wantErr string
		check   func(*testing.T, config)
	}{
		"defaults": {
			env: map[string]string{
				"STARCO_URL":           "https://starco.example.test",
				"STARCO_USERNAME":      "user@example.com",
				"STARCO_PASSWORD":      "secret",
				"STARCO_CHROMIUM_PATH": chromium,
			},
			check: func(t *testing.T, cfg config) {
				if cfg.Timeout != 90*time.Second {
					t.Errorf("Timeout = %s, want 90s", cfg.Timeout)
				}
				if cfg.ClockInSelector != "#clock-in" {
					t.Errorf("ClockInSelector = %q, want #clock-in", cfg.ClockInSelector)
				}
				if cfg.ClockOutSelector != "#clock-out" {
					t.Errorf("ClockOutSelector = %q, want #clock-out", cfg.ClockOutSelector)
				}
				if cfg.GeoLatitude != nil || cfg.GeoLongitude != nil {
					t.Errorf("GeoLatitude/GeoLongitude = %v/%v, want nil/nil", cfg.GeoLatitude, cfg.GeoLongitude)
				}
			},
		},
		"overrides": {
			env: map[string]string{
				"STARCO_URL":             "https://starco.example.test",
				"STARCO_USERNAME":        "user@example.com",
				"STARCO_PASSWORD":        "secret",
				"STARCO_CHROMIUM_PATH":   chromium,
				"STARCO_TIMEOUT_SECONDS": "120",
				"STARCO_GEO_LATITUDE":    "-6.2",
				"STARCO_GEO_LONGITUDE":   "106.816666",
			},
			check: func(t *testing.T, cfg config) {
				if cfg.Timeout != 120*time.Second {
					t.Errorf("Timeout = %s, want 120s", cfg.Timeout)
				}
				if cfg.GeoLatitude == nil || *cfg.GeoLatitude != -6.2 {
					t.Errorf("GeoLatitude = %v, want -6.2", cfg.GeoLatitude)
				}
				if cfg.GeoLongitude == nil || *cfg.GeoLongitude != 106.816666 {
					t.Errorf("GeoLongitude = %v, want 106.816666", cfg.GeoLongitude)
				}
			},
		},
		"missing username": {
			env: map[string]string{
				"STARCO_URL":           "https://starco.example.test",
				"STARCO_PASSWORD":      "secret",
				"STARCO_CHROMIUM_PATH": chromium,
			},
			wantErr: "STARCO_USERNAME is required",
		},
		"missing password": {
			env: map[string]string{
				"STARCO_URL":           "https://starco.example.test",
				"STARCO_USERNAME":      "user@example.com",
				"STARCO_CHROMIUM_PATH": chromium,
			},
			wantErr: "STARCO_PASSWORD is required",
		},
		"timeout below range": {
			env: map[string]string{
				"STARCO_URL":             "https://starco.example.test",
				"STARCO_USERNAME":        "user@example.com",
				"STARCO_PASSWORD":        "secret",
				"STARCO_CHROMIUM_PATH":   chromium,
				"STARCO_TIMEOUT_SECONDS": "5",
			},
			wantErr: "between 10 and 300",
		},
		"chromium missing": {
			env: map[string]string{
				"STARCO_URL":           "https://starco.example.test",
				"STARCO_USERNAME":      "user@example.com",
				"STARCO_PASSWORD":      "secret",
				"STARCO_CHROMIUM_PATH": "/nonexistent/chromium",
				"PATH":                 emptyPath,
			},
			wantErr: "chromium executable not found",
		},
		"geolocation without longitude": {
			env: map[string]string{
				"STARCO_URL":           "https://starco.example.test",
				"STARCO_USERNAME":      "user@example.com",
				"STARCO_PASSWORD":      "secret",
				"STARCO_CHROMIUM_PATH": chromium,
				"STARCO_GEO_LATITUDE":  "-6.2",
			},
			wantErr: "must be set together",
		},
		"geolocation not numeric": {
			env: map[string]string{
				"STARCO_URL":           "https://starco.example.test",
				"STARCO_USERNAME":      "user@example.com",
				"STARCO_PASSWORD":      "secret",
				"STARCO_CHROMIUM_PATH": chromium,
				"STARCO_GEO_LATITUDE":  "south",
				"STARCO_GEO_LONGITUDE": "106.8",
			},
			wantErr: "STARCO_GEO_LATITUDE must be a number",
		},
		"missing Starco URL": {
			env: map[string]string{
				"STARCO_USERNAME":      "user@example.com",
				"STARCO_PASSWORD":      "secret",
				"STARCO_CHROMIUM_PATH": chromium,
			},
			wantErr: "STARCO_URL is required",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			cfg, err := loadConfig()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("loadConfig() error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadConfig() unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}
}

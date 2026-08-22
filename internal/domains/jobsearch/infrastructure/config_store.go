package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type JSONAlertConfigStore struct{ path string }

func NewJSONAlertConfigStore(path string) *JSONAlertConfigStore {
	return &JSONAlertConfigStore{path: path}
}

func (s *JSONAlertConfigStore) Save(_ context.Context, config domain.AlertConfig) error {
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create job alert config directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".job-alert-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary job alert config: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure job alert config: %w", err)
	}
	persisted := struct {
		Query     string    `json:"query"`
		UpdatedAt time.Time `json:"updated_at"`
	}{Query: config.Query, UpdatedAt: config.UpdatedAt}
	if err := json.NewEncoder(temporary).Encode(persisted); err != nil {
		temporary.Close()
		return fmt.Errorf("encode job alert config: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync job alert config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close job alert config: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace job alert config: %w", err)
	}
	return nil
}

func (s *JSONAlertConfigStore) Load(_ context.Context) (domain.AlertConfig, error) {
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.AlertConfig{}, domain.ErrAlertConfigNotFound
	}
	if err != nil {
		return domain.AlertConfig{}, fmt.Errorf("open job alert config: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 4097))
	decoder.DisallowUnknownFields()
	var config domain.AlertConfig
	if err := decoder.Decode(&config); err != nil {
		return domain.AlertConfig{}, fmt.Errorf("decode job alert config: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return domain.AlertConfig{}, fmt.Errorf("decode job alert config: multiple JSON values")
	}
	return config, nil
}

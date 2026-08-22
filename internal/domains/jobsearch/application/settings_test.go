package application

import (
	"context"
	"errors"
	"testing"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type fakeAlertConfigStore struct {
	config domain.AlertConfig
	err    error
}

func (s *fakeAlertConfigStore) Save(_ context.Context, config domain.AlertConfig) error {
	s.config = config
	return s.err
}

func (s *fakeAlertConfigStore) Load(context.Context) (domain.AlertConfig, error) {
	return s.config, s.err
}

func TestSettingsServiceUpdatesAndLoadsScheduledCriteria(t *testing.T) {
	store := &fakeAlertConfigStore{}
	service := NewSettingsService(store)
	config, err := service.Update(t.Context(), "Software Engineer | go, typescript | 1 - 3 | Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	if !config.Criteria.Halal || config.Criteria.Interactive || config.Criteria.MaxYears != 3 {
		t.Fatalf("updated config=%#v", config)
	}
	criteria, err := service.ScheduledCriteria(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !criteria.Halal || len(criteria.Positions) != 1 || criteria.Positions[0] != "Software Engineer" {
		t.Fatalf("scheduled criteria=%#v", criteria)
	}
}

func TestSettingsServiceScheduledFallbackKeepsHalal(t *testing.T) {
	service := NewSettingsService(&fakeAlertConfigStore{err: domain.ErrAlertConfigNotFound})
	criteria, err := service.ScheduledCriteria(t.Context())
	if err != nil || !criteria.Halal {
		t.Fatalf("criteria=%#v err=%v", criteria, err)
	}
}

func TestSettingsServiceRejectsMissingPosition(t *testing.T) {
	_, err := NewSettingsService(&fakeAlertConfigStore{}).Update(t.Context(), " | go | 1-3 | Jakarta")
	if !errors.Is(err, domain.ErrInvalidAlertConfig) {
		t.Fatalf("err=%v", err)
	}
}

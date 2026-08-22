package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type AlertConfigStore interface {
	Save(context.Context, domain.AlertConfig) error
	Load(context.Context) (domain.AlertConfig, error)
}

type SettingsService struct{ store AlertConfigStore }

func NewSettingsService(store AlertConfigStore) *SettingsService {
	return &SettingsService{store: store}
}

func (s *SettingsService) Update(ctx context.Context, query string) (domain.AlertConfig, error) {
	query = strings.TrimSpace(query)
	criteria := domain.ParseQuery(query)
	if query == "" || len(criteria.Positions) == 0 {
		return domain.AlertConfig{}, fmt.Errorf("%w: posisi wajib diisi", domain.ErrInvalidAlertConfig)
	}
	criteria.Halal = true
	criteria.Interactive = false
	config := domain.AlertConfig{Query: query, Criteria: criteria, UpdatedAt: time.Now().UTC()}
	if err := s.store.Save(ctx, config); err != nil {
		return domain.AlertConfig{}, err
	}
	return config, nil
}

func (s *SettingsService) Current(ctx context.Context) (domain.AlertConfig, error) {
	config, err := s.store.Load(ctx)
	if err != nil {
		return domain.AlertConfig{}, err
	}
	criteria := domain.ParseQuery(config.Query)
	if len(criteria.Positions) == 0 {
		return domain.AlertConfig{}, fmt.Errorf("%w: posisi wajib diisi", domain.ErrInvalidAlertConfig)
	}
	criteria.Halal = true
	criteria.Interactive = false
	config.Criteria = criteria
	return config, nil
}

func (s *SettingsService) ScheduledCriteria(ctx context.Context) (domain.Criteria, error) {
	config, err := s.Current(ctx)
	if errors.Is(err, domain.ErrAlertConfigNotFound) {
		return domain.Criteria{Halal: true}, nil
	}
	if err != nil {
		return domain.Criteria{}, err
	}
	return config.Criteria, nil
}

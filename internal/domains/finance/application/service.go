package application

import (
	"context"
	"time"

	"ai-assistant/internal/domains/finance/domain"
)

type Repository interface {
	Create(context.Context, domain.RecordInput, time.Time) (domain.Record, error)
	Totals(context.Context, string) (domain.Totals, error)
	Records(context.Context, string) ([]domain.Record, error)
	Ping(context.Context) error
}

type Syncer interface {
	Sync(context.Context, domain.Record) (domain.SyncStatus, error)
}

type Service struct {
	repository Repository
	syncer     Syncer
}

func NewService(repository Repository, syncer Syncer) *Service {
	return &Service{repository: repository, syncer: syncer}
}

func (s *Service) Create(ctx context.Context, input domain.RecordInput) (domain.Record, domain.SyncStatus, error) {
	if err := input.Validate(); err != nil {
		return domain.Record{}, "", err
	}
	record, err := s.repository.Create(ctx, input, time.Now().UTC())
	if err != nil {
		return domain.Record{}, "", err
	}
	status := domain.SyncDisabled
	if s.syncer != nil {
		status, err = s.syncer.Sync(ctx, record)
		if err != nil {
			status = domain.SyncPending
		}
	}
	return record, status, nil
}

func (s *Service) Totals(ctx context.Context, phone string) (domain.Totals, error) {
	return s.repository.Totals(ctx, phone)
}

func (s *Service) Records(ctx context.Context, phone string) ([]domain.Record, error) {
	return s.repository.Records(ctx, phone)
}

func (s *Service) Ping(ctx context.Context) error { return s.repository.Ping(ctx) }

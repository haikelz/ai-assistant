package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type Ingestor interface {
	Run(context.Context, domain.Criteria, bool) (PipelineResult, error)
}

type Classifier interface {
	Classify(context.Context, []domain.NormalizedJob, domain.Criteria) ([]domain.Classification, error)
}

type Matcher interface {
	Rank([]domain.NormalizedJob, []domain.Classification, domain.Criteria) []domain.MatchResult
}

type AlertRunStore interface {
	StartRun(context.Context, domain.AlertRun) error
	FinishRun(context.Context, domain.AlertRun) error
}

type CuratedService struct {
	ingestor   Ingestor
	classifier Classifier
	matcher    Matcher
	runs       AlertRunStore
	messenger  Messenger
	logger     *log.Logger
	now        func() time.Time
}

func NewCuratedService(ingestor Ingestor, classifier Classifier, matcher Matcher, runs AlertRunStore, messenger Messenger, logger *log.Logger) *CuratedService {
	return &CuratedService{ingestor: ingestor, classifier: classifier, matcher: matcher, runs: runs, messenger: messenger, logger: logger, now: time.Now}
}

func (s *CuratedService) Run(ctx context.Context, criteria domain.Criteria, deliver, deduplicate bool) (message string, run domain.AlertRun, err error) {
	run = domain.AlertRun{ID: newRunID(), Status: domain.AlertRunRunning, StartedAt: s.now().UTC()}
	if err = s.runs.StartRun(ctx, run); err != nil {
		return "", run, fmt.Errorf("start alert run: %w", err)
	}
	defer func() {
		finishedAt := s.now().UTC()
		run.FinishedAt = &finishedAt
		if err != nil {
			run.Status = domain.AlertRunFailed
			run.ErrorMessage = err.Error()
		} else {
			run.Status = domain.AlertRunCompleted
		}
		if finishErr := s.runs.FinishRun(context.WithoutCancel(ctx), run); finishErr != nil {
			err = errors.Join(err, fmt.Errorf("finish alert run: %w", finishErr))
		}
	}()

	ingested, err := s.ingestor.Run(ctx, criteria, deduplicate)
	if err != nil {
		return "", run, fmt.Errorf("ingest jobs: %w", err)
	}
	run.JobsFetched = ingested.Fetched
	run.JobsNew = ingested.New
	run.JobsUpdated = ingested.Updated
	run.JobsFiltered = len(ingested.Jobs)

	classifications, classifyErr := s.classifier.Classify(ctx, ingested.Jobs, criteria)
	if classifyErr != nil {
		run.ErrorMessage = "AI classification fallback: " + classifyErr.Error()
		if s.logger != nil {
			s.logger.Printf("jobsearch: AI batch classification: %v", classifyErr)
		}
	}
	run.JobsClassified = len(classifications)
	matches := s.matcher.Rank(ingested.Jobs, classifications, criteria)
	run.JobsMatched = len(matches)
	message = FormatDigest(matches)
	if deliver && s.messenger != nil {
		if err = s.messenger.Send(ctx, message); err != nil {
			return message, run, fmt.Errorf("deliver digest: %w", err)
		}
	}
	return message, run, nil
}

func newRunID() string {
	var random [8]byte
	if _, err := rand.Read(random[:]); err == nil {
		return "run_" + hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("run_%d", time.Now().UnixNano())
}

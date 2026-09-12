package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

const (
	linkedInSearchURL       = "https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search"
	linkedInDetailURL       = "https://www.linkedin.com/jobs-guest/jobs/api/jobPosting/"
	linkedInDefaultLocation = "Indonesia"
	linkedInUserAgent       = "ai-assistant-job-alert/1.0 (personal use; public jobs only)"
)

type LinkedInConfig struct {
	SearchURL, DetailURL      string
	Pages, MaxDetails         int
	MaxQueries, Distance      int
	PostedWithin, MinInterval time.Duration
	JobTypes, CompanyIDs      []string
}

type LinkedIn struct {
	client *http.Client
	config LinkedInConfig

	mutex       sync.Mutex
	lastRequest time.Time
	blocked     bool
}

func NewLinkedIn(client *http.Client, config LinkedInConfig) (*LinkedIn, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if config.SearchURL == "" {
		config.SearchURL = linkedInSearchURL
	}
	if config.DetailURL == "" {
		config.DetailURL = linkedInDetailURL
	}
	if err := validateLinkedInURL("search", config.SearchURL); err != nil {
		return nil, err
	}
	if err := validateLinkedInURL("detail", config.DetailURL); err != nil {
		return nil, err
	}
	if config.Pages == 0 {
		config.Pages = 2
	}
	if config.Pages < 1 || config.Pages > 4 {
		return nil, fmt.Errorf("LinkedIn pages must be between 1 and 4")
	}
	if config.MaxDetails < 0 || config.MaxDetails > 10 {
		return nil, fmt.Errorf("LinkedIn detail limit must be between 0 and 10")
	}
	if config.MaxQueries == 0 {
		config.MaxQueries = 5
	}
	if config.MaxQueries < 1 || config.MaxQueries > 10 {
		return nil, fmt.Errorf("LinkedIn query limit must be between 1 and 10")
	}
	if config.Distance == 0 {
		config.Distance = 25
	}
	if config.Distance < 1 || config.Distance > 100 {
		return nil, fmt.Errorf("LinkedIn distance must be between 1 and 100 kilometers")
	}
	if config.PostedWithin == 0 {
		config.PostedWithin = 7 * 24 * time.Hour
	}
	if config.PostedWithin < time.Hour || config.PostedWithin > 30*24*time.Hour {
		return nil, fmt.Errorf("LinkedIn posted-within duration must be between 1 hour and 30 days")
	}
	if config.MinInterval < 0 {
		return nil, fmt.Errorf("LinkedIn request interval cannot be negative")
	}
	jobTypes, err := normalizeLinkedInJobTypes(config.JobTypes)
	if err != nil {
		return nil, err
	}
	companyIDs, err := normalizeLinkedInCompanyIDs(config.CompanyIDs)
	if err != nil {
		return nil, err
	}
	config.JobTypes, config.CompanyIDs = jobTypes, companyIDs
	config.SearchURL = strings.TrimRight(config.SearchURL, "/")
	config.DetailURL = strings.TrimRight(config.DetailURL, "/") + "/"
	return &LinkedIn{client: client, config: config}, nil
}

func (*LinkedIn) Source() string {
	return "linkedin"
}

func (*LinkedIn) Name() string {
	return "linkedin"
}

func (*LinkedIn) SupportsQueries() bool {
	return true
}

func (l *LinkedIn) HealthCheck(ctx context.Context) error {
	_, err := l.Search(ctx, domain.SearchQuery{Keyword: "software engineer"})
	return err
}

func (l *LinkedIn) Fetch(ctx context.Context, criteria domain.Criteria) ([]domain.Job, error) {
	queries := linkedInQueries(criteria, l.config.MaxQueries)
	seen := make(map[string]bool)
	var rawJobs []domain.RawJob
	var searchErrors []error
	for _, query := range queries {
		jobs, err := l.Search(ctx, query)
		for _, job := range jobs {
			if !seen[job.ExternalID] {
				seen[job.ExternalID] = true
				rawJobs = append(rawJobs, job)
			}
		}
		if err != nil {
			searchErrors = append(searchErrors, err)
			if errors.Is(err, domain.ErrProviderBlocked) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				break
			}
		}
	}
	return linkedInLegacyJobs(rawJobs), errors.Join(searchErrors...)
}

func (l *LinkedIn) Search(ctx context.Context, query domain.SearchQuery) ([]domain.RawJob, error) {
	seen := make(map[string]bool)
	var jobs []domain.RawJob
	for page := 0; page < l.config.Pages; page++ {
		searchURL, err := l.buildSearchURL(query, page*25)
		if err != nil {
			return jobs, err
		}
		body, err := l.get(ctx, searchURL, 4<<20)
		if err != nil {
			return jobs, fmt.Errorf("search LinkedIn page %d: %w", page+1, err)
		}
		pageJobs, err := parseLinkedInSearch(body)
		if err != nil {
			return jobs, fmt.Errorf("parse LinkedIn page %d: %w", page+1, err)
		}
		if len(pageJobs) == 0 {
			break
		}
		for _, job := range pageJobs {
			if !seen[job.ExternalID] {
				seen[job.ExternalID] = true
				jobs = append(jobs, job)
			}
		}
	}

	detailLimit := l.config.MaxDetails
	if detailLimit > len(jobs) {
		detailLimit = len(jobs)
	}
	for index := 0; index < detailLimit; index++ {
		body, err := l.get(ctx, l.config.DetailURL+url.PathEscape(jobs[index].ExternalID), 8<<20)
		if err != nil {
			return jobs, fmt.Errorf("fetch LinkedIn job %s: %w", jobs[index].ExternalID, err)
		}
		if err := enrichLinkedInJob(&jobs[index], body); err != nil {
			return jobs, fmt.Errorf("parse LinkedIn job %s: %w", jobs[index].ExternalID, err)
		}
	}
	return jobs, nil
}

func (l *LinkedIn) buildSearchURL(query domain.SearchQuery, start int) (string, error) {
	parsed, err := url.Parse(l.config.SearchURL)
	if err != nil {
		return "", fmt.Errorf("parse LinkedIn search URL: %w", err)
	}
	values := parsed.Query()
	values.Set("keywords", strings.TrimSpace(query.Keyword))
	location := strings.TrimSpace(query.Location)
	if location == "" {
		location = linkedInDefaultLocation
	}
	values.Set("location", location)
	values.Set("distance", strconv.Itoa(l.config.Distance))
	values.Set("f_TPR", "r"+strconv.FormatInt(int64(l.config.PostedWithin/time.Second), 10))
	values.Set("sortBy", "DD")
	values.Set("start", strconv.Itoa(start))
	if levels := linkedInExperienceLevels(query.MaxYears); len(levels) > 0 {
		values.Set("f_E", strings.Join(levels, ","))
	}
	if workModes := linkedInWorkModes(query.WorkModes); len(workModes) > 0 {
		values.Set("f_WT", strings.Join(workModes, ","))
	}
	if len(l.config.JobTypes) > 0 {
		values.Set("f_JT", strings.Join(l.config.JobTypes, ","))
	}
	if len(l.config.CompanyIDs) > 0 {
		values.Set("f_C", strings.Join(l.config.CompanyIDs, ","))
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

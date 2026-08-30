package providers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

const (
	linkedInSearchURL = "https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search"
	linkedInDetailURL = "https://www.linkedin.com/jobs-guest/jobs/api/jobPosting/"
	linkedInUserAgent = "ai-assistant-job-alert/1.0 (personal use; public jobs only)"
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

func (*LinkedIn) Source() string        { return "linkedin" }
func (*LinkedIn) Name() string          { return "linkedin" }
func (*LinkedIn) SupportsQueries() bool { return true }

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
	if location := strings.TrimSpace(query.Location); location != "" {
		values.Set("location", location)
	}
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

func validateLinkedInURL(name, value string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("LinkedIn %s URL must be an absolute HTTP URL", name)
	}
	return nil
}

func normalizeLinkedInJobTypes(values []string) ([]string, error) {
	allowed := map[string]bool{"F": true, "P": true, "C": true, "T": true, "V": true, "I": true, "O": true}
	seen := make(map[string]bool)
	var normalized []string
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if !allowed[value] {
			return nil, fmt.Errorf("unsupported LinkedIn job type %q", value)
		}
		if !seen[value] {
			seen[value] = true
			normalized = append(normalized, value)
		}
	}
	return normalized, nil
}

func normalizeLinkedInCompanyIDs(values []string) ([]string, error) {
	seen := make(map[string]bool)
	var normalized []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, err := strconv.ParseUint(value, 10, 64); err != nil {
			return nil, fmt.Errorf("invalid LinkedIn company ID %q", value)
		}
		if !seen[value] {
			seen[value] = true
			normalized = append(normalized, value)
		}
	}
	return normalized, nil
}

func linkedInExperienceLevels(maxYears int) []string {
	switch {
	case maxYears <= 0:
		return nil
	case maxYears <= 1:
		return []string{"2"}
	case maxYears <= 3:
		return []string{"2", "3"}
	case maxYears <= 6:
		return []string{"2", "3", "4"}
	default:
		return []string{"3", "4", "5", "6"}
	}
}

func linkedInWorkModes(values []domain.WorkMode) []string {
	seen := make(map[string]bool)
	var modes []string
	for _, value := range values {
		code := ""
		switch value {
		case domain.WorkModeOnsite:
			code = "1"
		case domain.WorkModeRemote:
			code = "2"
		case domain.WorkModeHybrid:
			code = "3"
		}
		if code != "" && !seen[code] {
			seen[code] = true
			modes = append(modes, code)
		}
	}
	return modes
}

func linkedInQueries(criteria domain.Criteria, maxQueries int) []domain.SearchQuery {
	location := ""
	if len(criteria.Locations) > 0 {
		location = criteria.Locations[0]
	}
	seen := make(map[string]bool)
	var queries []domain.SearchQuery
	add := func(keyword string) {
		keyword = strings.TrimSpace(keyword)
		key := strings.ToLower(keyword + "|" + location)
		if keyword != "" && !seen[key] && len(queries) < maxQueries {
			seen[key] = true
			queries = append(queries, domain.SearchQuery{Keyword: keyword, Location: location, MaxYears: criteria.MaxYears, WorkModes: append([]domain.WorkMode(nil), criteria.WorkModes...)})
		}
	}
	for _, position := range criteria.Positions {
		add(position)
	}
	for _, position := range criteria.Positions {
		for _, skill := range criteria.Skills {
			add(position + " " + skill)
		}
	}
	if len(queries) == 0 {
		add("software engineer")
	}
	return queries
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

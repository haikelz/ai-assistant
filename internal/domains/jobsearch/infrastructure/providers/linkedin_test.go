package providers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func TestLinkedInSearchAppliesFiltersAndEnrichesPublicJobs(t *testing.T) {
	queries := make(chan url.Values, 2)
	userAgents := make(chan string, 2)
	detailPaths := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/search":
			queries <- request.URL.Query()
			userAgents <- request.Header.Get("User-Agent")
			if request.URL.Query().Get("start") == "0" {
				writeTestResponse(t, response, linkedInSearchFixture("123"))
			}
		case "/detail/123":
			detailPaths <- request.URL.Path
			writeTestResponse(t, response, linkedInDetailFixture)
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider, err := NewLinkedIn(server.Client(), LinkedInConfig{
		SearchURL: server.URL + "/search", DetailURL: server.URL + "/detail",
		Pages: 2, MaxDetails: 1, MaxQueries: 3, Distance: 50,
		PostedWithin: 7 * 24 * time.Hour, JobTypes: []string{"F", "C"}, CompanyIDs: []string{"101", "202"},
	})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := provider.Search(t.Context(), domain.SearchQuery{Keyword: "Software Engineer Go", Location: "Jakarta", MaxYears: 3, WorkModes: []domain.WorkMode{domain.WorkModeRemote, domain.WorkModeHybrid}})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs=%#v", jobs)
	}
	job := jobs[0]
	if job.ExternalID != "123" || job.Title != "Software Engineer" || job.Company != "PT Example" || job.Location != "Jakarta, Indonesia" {
		t.Fatalf("job=%#v", job)
	}
	if !strings.Contains(job.Description, "2-3 years") || job.ExperienceText != "Associate 2-3 years" || job.EmploymentType != "Full-time" || job.WorkMode != "hybrid" || job.SalaryText != "Rp12.000.000 - Rp16.000.000" {
		t.Fatalf("enriched job=%#v", job)
	}
	if job.PublishedAt == nil || job.PublishedAt.Format("2006-01-02") != "2026-08-25" {
		t.Fatalf("published_at=%v", job.PublishedAt)
	}
	firstQuery := <-queries
	secondQuery := <-queries
	if firstQuery.Get("keywords") != "Software Engineer Go" || firstQuery.Get("location") != "Jakarta" || firstQuery.Get("distance") != "50" || firstQuery.Get("f_TPR") != "r604800" || firstQuery.Get("sortBy") != "DD" || firstQuery.Get("f_E") != "2,3" || firstQuery.Get("f_WT") != "2,3" || firstQuery.Get("f_JT") != "F,C" || firstQuery.Get("f_C") != "101,202" || firstQuery.Get("start") != "0" {
		t.Fatalf("first query=%v", firstQuery)
	}
	if secondQuery.Get("start") != "25" {
		t.Fatalf("second query=%v", secondQuery)
	}
	if userAgent := <-userAgents; userAgent != linkedInUserAgent {
		t.Fatalf("user-agent=%q", userAgent)
	}
	<-userAgents
	if path := <-detailPaths; path != "/detail/123" {
		t.Fatalf("detail path=%q", path)
	}
}

func TestLinkedInReturnsPartialJobsAndStopsAfterAccessControl(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Query().Get("start") == "0" {
			writeTestResponse(t, response, linkedInSearchFixture("123"))
			return
		}
		response.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	provider, err := NewLinkedIn(server.Client(), LinkedInConfig{SearchURL: server.URL, DetailURL: server.URL + "/detail", Pages: 2})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := provider.Search(t.Context(), domain.SearchQuery{Keyword: "Engineer"})
	if len(jobs) != 1 || !errors.Is(err, domain.ErrProviderBlocked) {
		t.Fatalf("jobs=%#v err=%v", jobs, err)
	}
	if _, secondErr := provider.Search(t.Context(), domain.SearchQuery{Keyword: "Engineer"}); !errors.Is(secondErr, domain.ErrProviderBlocked) {
		t.Fatalf("second error=%v", secondErr)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests=%d", requests.Load())
	}
}

func TestLinkedInRecognizesBlockedStatusesAndChallengePages(t *testing.T) {
	tests := map[string]http.HandlerFunc{
		"unauthorized": func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusUnauthorized) },
		"forbidden":    func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusForbidden) },
		"rate limited": func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusTooManyRequests) },
		"request denied": func(response http.ResponseWriter, _ *http.Request) {
			response.WriteHeader(999)
		},
		"challenge": func(response http.ResponseWriter, _ *http.Request) {
			writeTestResponse(t, response, "<html><title>Security verification challenge</title></html>")
		},
	}
	for name, handler := range tests {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			defer server.Close()
			provider, err := NewLinkedIn(server.Client(), LinkedInConfig{SearchURL: server.URL, DetailURL: server.URL + "/detail", Pages: 1})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.Search(t.Context(), domain.SearchQuery{Keyword: "Engineer"})
			if !errors.Is(err, domain.ErrProviderBlocked) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestLinkedInRejectsInvalidConfiguration(t *testing.T) {
	tests := []LinkedInConfig{
		{SearchURL: "://invalid"},
		{Pages: 5},
		{MaxDetails: 11},
		{Distance: 101},
		{JobTypes: []string{"INVALID"}},
		{CompanyIDs: []string{"not-a-number"}},
	}
	for _, config := range tests {
		if _, err := NewLinkedIn(nil, config); err == nil {
			t.Fatalf("config=%#v", config)
		}
	}
}

func writeTestResponse(t *testing.T, response http.ResponseWriter, body string) {
	t.Helper()
	if _, err := fmt.Fprint(response, body); err != nil {
		t.Errorf("write HTTP response: %v", err)
	}
}

func linkedInSearchFixture(id string) string {
	return `<div class="base-card base-search-card job-search-card" data-entity-urn="urn:li:jobPosting:` + id + `">
  <a class="base-card__full-link" href="https://www.linkedin.com/jobs/view/software-engineer-` + id + `?position=1"></a>
  <h3 class="base-search-card__title"> Software Engineer </h3>
  <h4 class="base-search-card__subtitle"><a>PT Example</a></h4>
  <span class="job-search-card__location">Jakarta, Indonesia</span>
  <time class="job-search-card__listdate" datetime="2026-08-25"></time>
</div>`
}

const linkedInDetailFixture = `<section>
  <div class="show-more-less-html__markup">Build reliable Go services in a hybrid team. Requires 2-3 years of experience.</div>
  <div class="compensation__salary">Rp12.000.000 - Rp16.000.000</div>
  <ul class="description__job-criteria-list">
    <li class="description__job-criteria-item"><h3>Seniority level</h3><span class="description__job-criteria-text">Associate</span></li>
    <li class="description__job-criteria-item"><h3>Employment type</h3><span class="description__job-criteria-text">Full-time</span></li>
  </ul>
</section>`

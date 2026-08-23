package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

const glintsUserAgent = "ai-assistant-job-alert/1.0 (personal use; contact configured by operator)"

type Glints struct {
	client      *http.Client
	baseURL     string
	minInterval time.Duration
	mutex       sync.Mutex
	lastRequest time.Time
}

func NewGlints(client *http.Client, baseURL string, minInterval time.Duration) *Glints {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = "https://glints.com/id/en/lowongan-kerja"
	}
	if minInterval < 0 {
		minInterval = 0
	}
	return &Glints{client: client, baseURL: baseURL, minInterval: minInterval}
}

func (*Glints) Source() string        { return "glints" }
func (*Glints) SupportsQueries() bool { return false }
func (g *Glints) HealthCheck(ctx context.Context) error {
	_, err := g.Search(ctx, domain.SearchQuery{})
	return err
}

func (g *Glints) Search(ctx context.Context, _ domain.SearchQuery) ([]domain.RawJob, error) {
	if err := g.wait(ctx); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, g.baseURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", glintsUserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := g.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("%w: status %d", domain.ErrProviderBlocked, response.StatusCode)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Glints status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	lowerBody := strings.ToLower(string(body))
	if strings.Contains(lowerBody, "captcha") || strings.Contains(lowerBody, "cf-chl-") || strings.Contains(lowerBody, "verify you are human") {
		return nil, fmt.Errorf("%w: challenge page", domain.ErrProviderBlocked)
	}
	jobs := parseGlintsJSONLD(body, g.baseURL)
	if len(jobs) == 0 {
		jobs = parseGlintsLinks(body, g.baseURL)
	}
	if len(jobs) == 0 {
		return nil, domain.ErrProviderLayout
	}
	return jobs, nil
}

func (g *Glints) wait(ctx context.Context) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	wait := g.minInterval - time.Since(g.lastRequest)
	if !g.lastRequest.IsZero() && wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	g.lastRequest = time.Now()
	return nil
}

var jsonLDScript = regexp.MustCompile(`(?is)<script[^>]+type=["']application/ld\+json["'][^>]*>(.*?)</script>`)
var glintsLink = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']*/opportunities/jobs/[^"']+/([0-9a-fA-F-]{36})[^"']*)["'][^>]*>(.*?)</a>`)
var htmlTag = regexp.MustCompile(`<[^>]+>`)

type glintsItemList struct {
	ItemListElement []struct {
		Item glintsPosting `json:"item"`
	} `json:"itemListElement"`
}
type glintsPosting struct {
	Type                                    string `json:"@type"`
	Title, Description, URL, EmploymentType string
	Identifier                              struct {
		Value string `json:"value"`
	} `json:"identifier"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation struct {
		Address struct {
			AddressLocality string `json:"addressLocality"`
		} `json:"address"`
	} `json:"jobLocation"`
	BaseSalary struct {
		Value struct {
			MinValue, MaxValue float64
			UnitText           string
		} `json:"value"`
	} `json:"baseSalary"`
}

func parseGlintsJSONLD(body []byte, baseURL string) []domain.RawJob {
	var jobs []domain.RawJob
	for _, match := range jsonLDScript.FindAllSubmatch(body, -1) {
		var list glintsItemList
		if json.Unmarshal(match[1], &list) != nil {
			continue
		}
		for _, element := range list.ItemListElement {
			posting := element.Item
			if !strings.EqualFold(posting.Type, "JobPosting") || strings.TrimSpace(posting.Title) == "" {
				continue
			}
			jobs = append(jobs, domain.RawJob{Source: "glints", ExternalID: posting.Identifier.Value, URL: absoluteURL(baseURL, posting.URL), Title: posting.Title, Company: posting.HiringOrganization.Name, Description: posting.Description, Location: posting.JobLocation.Address.AddressLocality, SalaryText: salaryText(posting.BaseSalary.Value.MinValue, posting.BaseSalary.Value.MaxValue), EmploymentType: posting.EmploymentType, FetchedAt: time.Now().UTC()})
		}
	}
	return jobs
}

func parseGlintsLinks(body []byte, baseURL string) []domain.RawJob {
	seen := map[string]bool{}
	var jobs []domain.RawJob
	for _, match := range glintsLink.FindAllSubmatch(body, -1) {
		id := string(match[2])
		if seen[id] {
			continue
		}
		seen[id] = true
		title := strings.TrimSpace(html.UnescapeString(htmlTag.ReplaceAllString(string(match[3]), " ")))
		if title != "" {
			jobs = append(jobs, domain.RawJob{Source: "glints", ExternalID: id, URL: absoluteURL(baseURL, string(match[1])), Title: strings.Join(strings.Fields(title), " "), FetchedAt: time.Now().UTC()})
		}
	}
	return jobs
}

func absoluteURL(baseURL, reference string) string {
	base, baseErr := url.Parse(baseURL)
	ref, refErr := url.Parse(reference)
	if baseErr != nil || refErr != nil {
		return reference
	}
	return base.ResolveReference(ref).String()
}

func salaryText(minimum, maximum float64) string {
	if minimum == 0 && maximum == 0 {
		return ""
	}
	return "Rp " + strconv.FormatInt(int64(minimum), 10) + " - " + strconv.FormatInt(int64(maximum), 10)
}

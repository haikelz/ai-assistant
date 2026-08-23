package providers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func TestGlintsParsesPublicJSONLDWithoutSearchParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.RawQuery != "" {
			t.Fatalf("unexpected query=%q", request.URL.RawQuery)
		}
		fmt.Fprint(response, `<script type="application/ld+json">{"itemListElement":[{"item":{"@type":"JobPosting","identifier":{"value":"123"},"title":"Software Engineer","description":"Build systems","url":"/id/en/opportunities/jobs/software-engineer/123","hiringOrganization":{"name":"PT Example"},"jobLocation":{"address":{"addressLocality":"Jakarta Selatan"}},"employmentType":"FULL_TIME","baseSalary":{"value":{"minValue":12000000,"maxValue":16000000}}}}]}</script>`)
	}))
	defer server.Close()
	jobs, err := NewGlints(server.Client(), server.URL, 0).Search(t.Context(), domain.SearchQuery{Keyword: "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Company != "PT Example" || jobs[0].ExternalID != "123" {
		t.Fatalf("jobs=%#v", jobs)
	}
}

func TestGlintsStopsOnAccessControlsAndChallenges(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"rate limit": func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusTooManyRequests) },
		"challenge": func(response http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(response, `<title>Verify you are human</title>`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			defer server.Close()
			_, err := NewGlints(server.Client(), server.URL, 0).Search(t.Context(), domain.SearchQuery{})
			if !errors.Is(err, domain.ErrProviderBlocked) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

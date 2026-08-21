package infrastructure

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func TestKitalulusFetchParsesSSRCard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("keyword") != "Software Engineer" {
			t.Fatalf("keyword=%q", r.URL.Query().Get("keyword"))
		}
		fmt.Fprint(w, `<a href="/lowongan/detail/software-engineer"><h3>Software Engineer</h3><p>PT Example</p><p>Jakarta Selatan</p></a>`)
	}))
	defer server.Close()
	jobs, err := NewKitalulus(server.Client(), server.URL).Fetch(t.Context(), domain.Criteria{Positions: []string{"Software Engineer"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Title != "Software Engineer" || jobs[0].Company != "PT Example" || jobs[0].Location != "Jakarta Selatan" {
		t.Fatalf("jobs=%#v", jobs)
	}
}

func TestDeallsFetchParsesNextData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("search") != "Software Engineer" {
			t.Fatalf("search=%q", r.URL.Query().Get("search"))
		}
		fmt.Fprint(w, `<script id="__NEXT_DATA__">{"props":{"pageProps":{"dehydratedState":{"queries":[{"state":{"data":{"pages":[{"docs":[{"slug":"software-engineer","role":"Software Engineer","company":{"name":"PT Example","slug":"example"},"city":{"name":"Jakarta Selatan"},"skills":[{"name":"Go"}],"employmentTypes":["Full-time"]}] }]}}}]}}}}</script>`)
	}))
	defer server.Close()
	jobs, err := NewDealls(server.Client(), server.URL).Fetch(t.Context(), domain.Criteria{Positions: []string{"Software Engineer"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Company != "PT Example" || jobs[0].Skills != "Go" || jobs[0].Type != "Full-time" {
		t.Fatalf("jobs=%#v", jobs)
	}
}

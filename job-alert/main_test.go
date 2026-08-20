package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildSearchURL(t *testing.T) {
	got := buildSearchURL("https://example.com/jobs", "keyword", []string{"Software Engineer"})
	if got != "https://example.com/jobs?keyword=Software+Engineer" {
		t.Fatalf("search URL = %q", got)
	}
}

func TestParseKitalulusHTML(t *testing.T) {
	html := `<a href="/lowongan/detail/software-engineer-wd6r"><h3>Software Engineer</h3><p>PT Vistakom Infomedia</p><p>Jakarta Barat, DKI Jakarta</p><p>Minimal S1</p></a>`
	jobs := parseKitalulusHTML(html)
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].Title != "Software Engineer" || jobs[0].Company != "PT Vistakom Infomedia" || jobs[0].Location != "Jakarta Barat, DKI Jakarta" {
		t.Fatalf("incorrect parsed job: %#v", jobs[0])
	}
}

func TestFilterByKeywordOrSkill(t *testing.T) {
	jobs := []Job{
		{Title: "Software Developer", Skills: "Golang, TypeScript"},
		{Title: "Backend Golang Developer", Skills: "Golang"},
		{Title: "Software Engineer", Skills: "Java"},
		{Title: "Software Engineer"},
	}

	got := filterByKeywordOrSkill(jobs, []string{"Software Engineer"}, []string{"go", "typescript"})
	if len(got) != 2 {
		t.Fatalf("got %d jobs, want 2: %#v", len(got), got)
	}
	if got[0].Title != "Software Developer" {
		t.Errorf("first job = %q, want Software Developer", got[0].Title)
	}
	if got[1].Title != "Software Engineer" || got[1].Skills != "" {
		t.Errorf("missing-skill job should be retained, got %#v", got[1])
	}
}

func TestMatchesLocationNormalizesEnglishJakarta(t *testing.T) {
	original := targetLocations
	targetLocations = []string{"Jakarta Selatan"}
	t.Cleanup(func() { targetLocations = original })

	if !matchesLocation("South Jakarta, DKI Jakarta") {
		t.Fatal("South Jakarta should match Jakarta Selatan")
	}
}

func TestFormatMessageUsesActiveSources(t *testing.T) {
	message := formatMessage("Hasil", nil, nil)
	if strings.Contains(message, "Kalibrr") {
		t.Fatal("message should not include removed Kalibrr source")
	}
	if !strings.Contains(message, "A. Kitalulus") || !strings.Contains(message, "B. Dealls") {
		t.Fatalf("message has incorrect source sections: %q", message)
	}
}

func TestApplyCompanyAssessments(t *testing.T) {
	jobs := []Job{
		{Company: "Company A", HalalStatus: halalStatusNeedsReview},
		{Company: "Company B", HalalStatus: halalStatusNeedsReview},
	}
	applyCompanyAssessments([][]Job{jobs}, []companyAssessment{
		{Company: "company a", Status: halalStatusHalal},
		{Company: "Company B", Status: halalStatusNotHalal, Reason: "Bank"},
	})
	if jobs[0].HalalStatus != halalStatusHalal {
		t.Fatalf("Company A status = %q", jobs[0].HalalStatus)
	}
	if jobs[1].HalalStatus != halalStatusNotHalal || jobs[1].HalalReason != "Bank" {
		t.Fatalf("Company B assessment = %#v", jobs[1])
	}
}

func TestAssessHalalCompanies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"output":[{"content":[{"text":"[{\"company\":\"Company A\",\"status\":\"halal\",\"reason\":\"Bisnis perangkat lunak\"}]"}]}]}`)
	}))
	defer server.Close()
	t.Setenv("SUMOPOD_API_KEY", "test-key")
	t.Setenv("SUMOPOD_RESPONSES_URL", server.URL)

	jobs := []Job{{Company: "Company A", Title: "Software Engineer"}}
	assessHalalCompanies(server.Client(), jobs)
	if jobs[0].HalalStatus != halalStatusHalal || jobs[0].HalalReason != "Bisnis perangkat lunak" {
		t.Fatalf("assessment = %#v", jobs[0])
	}
}

func TestHalalLabel(t *testing.T) {
	tests := []struct {
		job  Job
		want string
	}{
		{Job{}, ""},
		{Job{HalalStatus: halalStatusHalal}, " (Halal)"},
		{Job{HalalStatus: halalStatusNotHalal, HalalReason: "Asuransi"}, " (Tidak Halal — Asuransi)"},
		{Job{HalalStatus: halalStatusNeedsReview}, " (Perlu Riset)"},
	}
	for _, test := range tests {
		if got := halalLabel(test.job); got != test.want {
			t.Errorf("halalLabel(%#v) = %q, want %q", test.job, got, test.want)
		}
	}
}

func TestSplitTelegramMessageKeepsAllContent(t *testing.T) {
	message := strings.Repeat("lowongan kerja\n", 400)
	chunks := splitTelegramMessage(message, 4000)
	if len(chunks) < 2 {
		t.Fatalf("got %d chunk, want multiple chunks", len(chunks))
	}
	if strings.Join(chunks, "") != message {
		t.Fatal("split chunks did not preserve the complete message")
	}
	for i, chunk := range chunks {
		if len(chunk) > 4000 {
			t.Errorf("chunk %d has %d bytes, want at most 4000", i, len(chunk))
		}
	}
}

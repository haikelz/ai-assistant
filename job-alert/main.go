package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	glintsGraphqlURL   = "https://glints.com/api/v2-alc/graphql"
	jobstreetSearchURL = "https://id.jobstreet.com/id/jobs"
	sumopodURL         = "https://ai.sumopod.com/v1/responses"
	telegramAPIURL     = "https://api.telegram.org"
	requestTimeout     = 30 * time.Second
	jobsPerSource      = 20
	jobsPerRequest     = 30
	glintsMaxPages     = 2
	jobstreetMaxPages  = 2
)

var keywords = []string{
	"fullstack developer", "fullstack engineer", "devops engineer",
	"frontend developer", "backend developer", "frontend engineer",
	"backend engineer", "product developer", "product engineer",
}

// ponytail: location filtering done by substring match on city/province.
// Upgrade path: use geocoding API for precise radius filtering.
var targetLocations = []string{
	"jakarta", "bogor", "depok", "tangerang", "bekasi",
	"karawang", "bandung", "cikampek", "cikarang",
	"solo", "surabaya", "bali", "batam", "salatiga",
	"surakarta", "denpasar",
	"dki jakarta", "jawa barat", "jawa timur", "jawa tengah", "banten",
	"kepulauan riau",
}
var techStack = []string{
	"node js", "node.js", "express js", "express.js", "nest js", "nestjs",
	"typescript", "javascript", "fastify", "next js", "next.js",
	"tailwindcss", "tailwind", "react query", "svelte", "svelte kit",
	"astro", "react js", "react.js", "reactjs", "postgresql", "postgres",
	"mysql", "golang", "go", "echo", "fiber", "gorm", "redis",
	"aws s3", "aws", "firebase", "supabase", "turso db", "turso",
	"monolith", "microservice", "ci/cd", "ci cd", "github", "gitlab",
	"linux", "ubuntu", "fedora", "kubernetes", "k8s", "docker",
	"argo cd", "argocd",
}

// maxYearsExp bounds the experience filter (0 = no upper bound).
var maxYearsExp = 3

// Job represents a normalized job listing from any source.
type Job struct {
	Title       string `json:"title"`
	Company     string `json:"company"`
	Location    string `json:"location"`
	URL         string `json:"url"`
	Source      string `json:"source"`
	Salary      string `json:"salary"`
	Type        string `json:"type"`
	Experience  string `json:"experience"`
	Skills      string `json:"skills"`
	PostedAt    string `json:"posted_at"`
	MinYearsExp int    `json:"min_years_exp"`
	MaxYearsExp int    `json:"max_years_exp"`
}

func main() {
	keywordsFlag := flag.String("keywords", "", "comma-separated search keywords (default: built-in list)")
	locationFlag := flag.String("location", "", "comma-separated target locations (default: built-in list)")
	skillsFlag := flag.String("skills", "", "comma-separated tech stack for AI filtering (default: built-in list)")
	experienceFlag := flag.String("experience", "", "max years of experience, e.g. 3 (default: 3)")
	dryRun := flag.Bool("dry-run", false, "print the message to stdout instead of sending to Telegram")
	flag.Parse()

	if *keywordsFlag != "" {
		keywords = splitAndTrim(*keywordsFlag)
	}
	if *locationFlag != "" {
		targetLocations = splitAndTrim(*locationFlag)
	}
	if *skillsFlag != "" {
		techStack = splitAndTrim(*skillsFlag)
	}
	if *experienceFlag != "" {
		fmt.Sscanf(strings.TrimSpace(*experienceFlag), "%d", &maxYearsExp)
	}

	client := &http.Client{Timeout: requestTimeout}

	glintsJobs := fetchGlintsJobs(client)
	jobstreetJobs := fetchJobstreetJobs(client)

	glintsFiltered := filterJobs(glintsJobs)
	jobstreetFiltered := filterJobs(jobstreetJobs)

	glintsFinal := aiFilterJobs(client, glintsFiltered)
	jsFinal := aiFilterJobs(client, jobstreetFiltered)
	sortJobsByRelevance(glintsFinal)
	sortJobsByRelevance(jsFinal)
	glintsFinal = limitJobs(glintsFinal, jobsPerSource)
	jsFinal = limitJobs(jsFinal, jobsPerSource)

	message := formatMessage(glintsFinal, jsFinal)
	if *dryRun {
		fmt.Println(message)
		return
	}
	sendTelegram(client, message)
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// --- Glints Fetcher ---

type glintsGraphQLRequest struct {
	OperationName string         `json:"operationName"`
	Variables     map[string]any `json:"variables"`
	Query         string         `json:"query"`
}

type glintsJob struct {
	ID                    string               `json:"id"`
	Title                 string               `json:"title"`
	MinYearsOfExperience  *int                 `json:"minYearsOfExperience"`
	MaxYearsOfExperience  *int                 `json:"maxYearsOfExperience"`
	Status                string               `json:"status"`
	CreatedAt             string               `json:"createdAt"`
	Type                  string               `json:"type"`
	WorkArrangementOption string               `json:"workArrangementOption"`
	EducationLevel        string               `json:"educationLevel"`
	ShouldShowSalary      bool                 `json:"shouldShowSalary"`
	Company               glintsCompany        `json:"company"`
	Location              glintsLocation       `json:"location"`
	Salaries              []glintsSalary       `json:"salaries"`
	Skills                []glintsSkillWrapper `json:"skills"`
}

type glintsCompany struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type glintsLocation struct {
	Name    string                 `json:"name"`
	Parents []glintsLocationParent `json:"parents"`
}

type glintsLocationParent struct {
	Name    string                      `json:"name"`
	Parents []glintsLocationGrandParent `json:"parents"`
}

type glintsLocationGrandParent struct {
	Name string `json:"name"`
}

type glintsSalary struct {
	MinAmount    int    `json:"minAmount"`
	MaxAmount    int    `json:"maxAmount"`
	SalaryMode   string `json:"salaryMode"`
	CurrencyCode string `json:"CurrencyCode"`
}

type glintsSkillWrapper struct {
	Skill glintsSkill `json:"skill"`
}

type glintsSkill struct {
	Name string `json:"name"`
}

type glintsSearchResponse struct {
	Data struct {
		SearchJobsV3 struct {
			JobsInPage []glintsJob `json:"jobsInPage"`
			HasMore    bool        `json:"hasMore"`
		} `json:"searchJobsV3"`
	} `json:"data"`
}

const glintsSearchQuery = `query searchJobsV3($data: JobSearchConditionInput!) {
  searchJobsV3(data: $data) {
    jobsInPage {
      id
      title
      status
      createdAt
      type
      workArrangementOption
      educationLevel
      shouldShowSalary
      company {
        id
        name
        __typename
      }
      location {
        name
        parents {
          name
          parents {
            name
            __typename
          }
          __typename
        }
        __typename
      }
      salaries {
        minAmount
        maxAmount
        salaryMode
        CurrencyCode
        __typename
      }
      minYearsOfExperience
      maxYearsOfExperience
      skills {
        skill {
          name
          __typename
        }
        __typename
      }
      __typename
    }
    hasMore
    __typename
  }
}`

func fetchGlintsJobs(client *http.Client) []Job {
	var all []Job
	seen := map[string]bool{}

	for _, keyword := range keywords {
		for page := 1; page <= glintsMaxPages; page++ {
			vars := map[string]any{
				"data": map[string]any{
					"SearchTerm":          keyword,
					"CountryCode":         "ID",
					"includeExternalJobs": true,
					"pageSize":            jobsPerRequest,
					"page":                page,
				},
			}

			req := glintsGraphQLRequest{
				OperationName: "searchJobsV3",
				Variables:     vars,
				Query:         glintsSearchQuery,
			}

			body, err := json.Marshal(req)
			if err != nil {
				continue
			}

			jobs, hasMore := doGlintsRequest(client, body)
			for _, j := range jobs {
				if seen[j.ID] {
					continue
				}
				seen[j.ID] = true
				all = append(all, glintsToJob(j))
			}

			if !hasMore {
				break
			}
		}
	}

	return all
}

func doGlintsRequest(client *http.Client, body []byte) ([]glintsJob, bool) {
	req, err := http.NewRequest(http.MethodPost, glintsGraphqlURL, bytes.NewReader(body))
	if err != nil {
		return nil, false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("x-glints-country-code", "ID")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; JobAlertBot/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false
	}

	var result glintsSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, false
	}

	return result.Data.SearchJobsV3.JobsInPage, result.Data.SearchJobsV3.HasMore
}

func glintsToJob(j glintsJob) Job {
	loc := buildLocationString(j.Location)
	salary := formatGlintsSalary(j.Salaries)
	exp := formatExperience(j.MinYearsOfExperience, j.MaxYearsOfExperience)
	skills := formatGlintsSkills(j.Skills)

	return Job{
		Title:       j.Title,
		Company:     j.Company.Name,
		Location:    loc,
		URL:         fmt.Sprintf("https://glints.com/id/opportunities/jobs/%s", j.ID),
		Source:      "glints",
		Salary:      salary,
		Type:        j.Type,
		Experience:  exp,
		Skills:      skills,
		PostedAt:    j.CreatedAt,
		MinYearsExp: derefInt(j.MinYearsOfExperience),
		MaxYearsExp: derefInt(j.MaxYearsOfExperience),
	}
}

func buildLocationString(loc glintsLocation) string {
	parts := []string{loc.Name}
	for _, p := range loc.Parents {
		if p.Name != "" && p.Name != "Indonesia" {
			parts = append(parts, p.Name)
		}
	}
	return strings.Join(parts, ", ")
}

func formatGlintsSalary(salaries []glintsSalary) string {
	if len(salaries) == 0 {
		return ""
	}
	s := salaries[0]
	if s.MinAmount == 0 && s.MaxAmount == 0 {
		return ""
	}
	mode := ""
	switch s.SalaryMode {
	case "MONTH":
		mode = "/bln"
	case "YEAR":
		mode = "/thn"
	}
	return fmt.Sprintf("Rp %s - %s jt%s", formatMillions(s.MinAmount), formatMillions(s.MaxAmount), mode)
}

func formatMillions(amount int) string {
	m := float64(amount) / 1_000_000
	if m == float64(int(m)) {
		return fmt.Sprintf("%.0f", m)
	}
	return fmt.Sprintf("%.1f", m)
}

func formatExperience(min, max *int) string {
	if min == nil && max == nil {
		return ""
	}
	if min != nil && max != nil {
		if *min == *max {
			return fmt.Sprintf("%d tahun", *min)
		}
		return fmt.Sprintf("%d–%d tahun", *min, *max)
	}
	if min != nil {
		return fmt.Sprintf("≥%d tahun", *min)
	}
	return fmt.Sprintf("≤%d tahun", *max)
}

func formatGlintsSkills(skills []glintsSkillWrapper) string {
	names := make([]string, 0, len(skills))
	for _, s := range skills {
		names = append(names, s.Skill.Name)
	}
	return strings.Join(names, ", ")
}

// --- Jobstreet Fetcher ---

// Jobstreet (SEEK) serves search results as server-side-rendered HTML.
// Each job card carries data-automation="normalJob" with structured fields.
// ponytail: regex on SSR HTML; ceiling = breaks if SEEK changes card markup.
// Upgrade path: call the candidate GraphQL endpoint (candidate-graphql-dark-pro.cloud.seek.com.au).

var (
	jobstreetCardRe     = regexp.MustCompile(`data-automation="normalJob"`)
	jobstreetIDRe       = regexp.MustCompile(`data-job-id="(\d+)"`)
	jobstreetTitleRe    = regexp.MustCompile(`aria-label="([^"]+)"`)
	jobstreetFieldRe    = regexp.MustCompile(`data-automation="(jobCompany|jobLocation|jobSalary|jobListingDate|jobClassification)"[^>]*>(.*?)</(?:a|span|div)>`)
	jobstreetWorkTypeRe = regexp.MustCompile(`Ini adalah lowongan kerja ([^<]+)`)
	htmlTagRe           = regexp.MustCompile(`<[^>]*>`)
)

func fetchJobstreetJobs(client *http.Client) []Job {
	var all []Job
	seen := map[string]bool{}

	for _, keyword := range keywords {
		for page := 1; page <= jobstreetMaxPages; page++ {
			jobs := fetchJobstreetPage(client, keyword, page)
			if len(jobs) == 0 {
				break
			}
			for _, j := range jobs {
				if seen[j.URL] {
					continue
				}
				seen[j.URL] = true
				all = append(all, j)
			}
		}
	}

	return all
}

func fetchJobstreetPage(client *http.Client, keyword string, page int) []Job {
	url := fmt.Sprintf("%s?keywords=%s&page=%d",
		jobstreetSearchURL,
		strings.ReplaceAll(keyword, " ", "+"),
		page,
	)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "id-ID,id;q=0.9")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	return parseJobstreetHTML(body)
}

func parseJobstreetHTML(html []byte) []Job {
	text := string(html)

	// Split into job cards by the card marker.
	cardIndices := jobstreetCardRe.FindAllStringIndex(text, -1)
	if len(cardIndices) == 0 {
		return nil
	}

	jobs := make([]Job, 0, len(cardIndices))
	for i, loc := range cardIndices {
		start := loc[0]
		end := len(text)
		if i+1 < len(cardIndices) {
			end = cardIndices[i+1][0]
		}
		card := text[start:end]

		job := parseJobstreetCard(card)
		if job.Title != "" && job.URL != "" {
			jobs = append(jobs, job)
		}
	}

	return jobs
}

func parseJobstreetCard(card string) Job {
	var job Job
	job.Source = "jobstreet"

	if m := jobstreetIDRe.FindStringSubmatch(card); len(m) > 1 {
		job.URL = fmt.Sprintf("https://id.jobstreet.com/id/job/%s", m[1])
	}
	if m := jobstreetTitleRe.FindStringSubmatch(card); len(m) > 1 {
		job.Title = decodeHTML(m[1])
	}

	for _, m := range jobstreetFieldRe.FindAllStringSubmatch(card, -1) {
		field := m[1]
		value := decodeHTML(stripTags(m[2]))
		switch field {
		case "jobCompany":
			job.Company = value
		case "jobLocation":
			job.Location = value
		case "jobSalary":
			job.Salary = value
		case "jobListingDate":
			job.PostedAt = value
		}
	}

	if m := jobstreetWorkTypeRe.FindStringSubmatch(card); len(m) > 1 {
		job.Type = decodeHTML(m[1])
	}

	return job
}

func stripTags(s string) string {
	return htmlTagRe.ReplaceAllString(s, "")
}

func decodeHTML(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	return strings.TrimSpace(s)
}

// --- Location Filtering ---

func matchesLocation(location string) bool {
	lower := strings.ToLower(location)
	for _, loc := range targetLocations {
		if strings.Contains(lower, loc) {
			return true
		}
	}
	return false
}

// --- Experience Filtering ---

func matchesExperience(j Job) bool {
	if j.MinYearsExp == 0 && j.MaxYearsExp == 0 {
		return true // unknown, include for AI to judge
	}
	if maxYearsExp > 0 && j.MinYearsExp > maxYearsExp {
		return false
	}
	return true
}

// --- Combined Filtering ---

func filterJobs(jobs []Job) []Job {
	var filtered []Job
	for _, j := range jobs {
		if matchesLocation(j.Location) && matchesExperience(j) {
			filtered = append(filtered, j)
		}
	}
	return filtered
}

func limitJobs(jobs []Job, limit int) []Job {
	if len(jobs) <= limit {
		return jobs
	}
	return jobs[:limit]
}

// --- AI Filtering ---

// Sumopod exposes the OpenAI Responses API (not chat completions).
// Request: {model, instructions, input}; response: output[].content[].text.

type responsesRequest struct {
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
	Input        string `json:"input"`
}

type responsesContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesOutputItem struct {
	Type    string             `json:"type"`
	Content []responsesContent `json:"content"`
}

type responsesResponse struct {
	Output []responsesOutputItem `json:"output"`
}

func aiFilterJobs(client *http.Client, jobs []Job) []Job {
	if len(jobs) == 0 {
		return nil
	}

	jobsJSON, _ := json.Marshal(jobs)

	instructions := fmt.Sprintf(`Kamu adalah filter lowongan kerja. Analisis setiap job listing JSON dan RETURN HANYA array JSON dari job yang match kriteria:

TECH STACK (minimal 1 match):
%s

PENGALAMAN KERJA: 1-3 tahun (toleransi jika tidak disebutkan atau <=3 tahun).

LOKASI: jabodetabek, bandung, surabaya, bali, batam, solo, salatiga, karawang, cikampek, cikarang.

Rules:
- Return HANYA JSON array, tanpa teks lain, tanpa markdown.
- Jangan ubah struktur object job, kembalikan object aslinya persis.
- Kalau tidak ada yang match, return [].
- Maksimal 30 job.`, strings.Join(techStack, ", "))

	req := responsesRequest{
		Model:        "deepseek-v4-pro",
		Instructions: instructions,
		Input:        string(jobsJSON),
	}

	body, err := json.Marshal(req)
	if err != nil {
		return jobs
	}

	httpReq, err := http.NewRequest(http.MethodPost, sumopodURL, bytes.NewReader(body))
	if err != nil {
		return jobs
	}

	apiKey := os.Getenv("SUMOPOD_API_KEY")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return jobs
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return jobs
	}

	var rresp responsesResponse
	if err := json.NewDecoder(resp.Body).Decode(&rresp); err != nil {
		return jobs
	}

	content := extractResponsesText(rresp)
	content = cleanJSONResponse(content)

	var filtered []Job
	if err := json.Unmarshal([]byte(content), &filtered); err != nil || len(filtered) == 0 {
		// ponytail: AI returned nothing or unparseable — keep the pre-filtered list.
		// Ceiling: unfiltered jobs may include non-matching tech stacks. Upgrade: deterministic skill filter.
		return jobs
	}

	return filtered
}

func extractResponsesText(r responsesResponse) string {
	for _, item := range r.Output {
		for _, c := range item.Content {
			if c.Text != "" {
				return c.Text
			}
		}
	}
	return ""
}

// cleanJSONResponse extracts JSON array from AI response that may have markdown wrappers.
func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "```json"); idx >= 0 {
		s = s[idx+7:]
	} else if idx := strings.Index(s, "```"); idx >= 0 {
		s = s[idx+3:]
	}
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	return s
}

// --- Message Formatter ---

func formatMessage(glintsJobs, jobstreetJobs []Job) string {
	var b strings.Builder

	b.WriteString("Selamat pagi! ☀️ Berikut update lowongan kerja terbaru hari ini:\n\n")
	b.WriteString("Daftar Job Terbaru:\n\n")

	b.WriteString("A. Glints\n\n")
	if len(glintsJobs) == 0 {
		b.WriteString("Tidak ada lowongan baru yang sesuai kriteria.\n\n")
	} else {
		for i, j := range glintsJobs {
			writeJobEntry(&b, i+1, j)
		}
	}

	b.WriteString("\nB. Jobstreet\n\n")
	if len(jobstreetJobs) == 0 {
		b.WriteString("Tidak ada lowongan baru yang sesuai kriteria.\n\n")
	} else {
		for i, j := range jobstreetJobs {
			writeJobEntry(&b, i+1, j)
		}
	}

	b.WriteString("\n— Dikirim otomatis oleh Job Alert Bot")

	return b.String()
}

func writeJobEntry(b *strings.Builder, num int, j Job) {
	b.WriteString(fmt.Sprintf("%d. **%s**\n", num, j.Title))
	if j.Company != "" {
		b.WriteString(fmt.Sprintf("   🏢 %s\n", j.Company))
	}
	if j.Location != "" {
		b.WriteString(fmt.Sprintf("   📍 %s\n", j.Location))
	}
	if j.Salary != "" {
		b.WriteString(fmt.Sprintf("   💰 %s\n", j.Salary))
	}
	if j.Type != "" {
		b.WriteString(fmt.Sprintf("   📋 %s\n", j.Type))
	}
	if j.Experience != "" {
		b.WriteString(fmt.Sprintf("   🎯 %s\n", j.Experience))
	}
	if j.Skills != "" {
		b.WriteString(fmt.Sprintf("   🛠 %s\n", j.Skills))
	}
	b.WriteString(fmt.Sprintf("   🔗 %s\n\n", j.URL))
}

// --- Telegram Sender ---

func sendTelegram(client *http.Client, message string) {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_USER_ID")

	if botToken == "" || chatID == "" {
		fmt.Fprintln(os.Stderr, "TELEGRAM_BOT_TOKEN or TELEGRAM_USER_ID not set, printing message to stdout instead.")
		fmt.Println(message)
		return
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", telegramAPIURL, botToken)

	payload := map[string]string{
		"chat_id":                  chatID,
		"text":                     message,
		"parse_mode":               "Markdown",
		"disable_web_page_preview": "true",
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// Truncate if too long for Telegram (4096 chars)
	if len(message) > 4000 {
		payload["text"] = message[:4000] + "\n\n— (dipotong karena terlalu panjang)"
		body, _ = json.Marshal(payload)
		req, _ = http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to send telegram message: %v\n", err)
		// ponytail: fallback to stdout on error
		// Ceiling: if Telegram is down, message is lost. Upgrade path: add retry queue.
		fmt.Println(message)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "telegram API error (%d): %s\n", resp.StatusCode, string(respBody))
		fmt.Println(message)
	}
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// sortJobsByRelevance sorts by number of matching tech stack skills (ponytail: O(n*s), fine for n≤100).
func sortJobsByRelevance(jobs []Job) {
	sort.Slice(jobs, func(i, j int) bool {
		return countTechMatches(jobs[i]) > countTechMatches(jobs[j])
	})
}

func countTechMatches(j Job) int {
	count := 0
	lower := strings.ToLower(j.Skills + " " + j.Title)
	for _, tech := range techStack {
		if strings.Contains(lower, tech) {
			count++
		}
	}
	return count
}

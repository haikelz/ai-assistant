package main
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)
const (
	glintsGraphqlURL     = "https://glints.com/api/v2-alc/graphql"
	jobstreetSearchURL   = "https://www.jobstreet.co.id/api/jobs/search"
	sumopodChatURL       = "https://ai.sumopod.com/v1/chat/completions"
	telegramAPIURL       = "https://api.telegram.org"
	requestTimeout       = 45 * time.Second
	jobsPerSource        = 20
	jobsPerRequest       = 30
	glintsMaxPages       = 3
	jobstreetMaxPages    = 3
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

// Job represents a normalized job listing from any source.
type Job struct {
	Title        string `json:"title"`
	Company      string `json:"company"`
	Location     string `json:"location"`
	URL          string `json:"url"`
	Source       string `json:"source"`
	Salary       string `json:"salary"`
	Type         string `json:"type"`
	Experience   string `json:"experience"`
	Skills       string `json:"skills"`
	PostedAt     string `json:"posted_at"`
	MinYearsExp  int    `json:"min_years_exp"`
	MaxYearsExp  int    `json:"max_years_exp"`
}

func main() {
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
	sendTelegram(client, message)
}

// --- Glints Fetcher ---

type glintsGraphQLRequest struct {
	OperationName string      `json:"operationName"`
	Variables     map[string]any `json:"variables"`
	Query         string      `json:"query"`
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
	Name    string               `json:"name"`
	Parents []glintsLocationParent `json:"parents"`
}

type glintsLocationParent struct {
	Name    string                    `json:"name"`
	Parents []glintsLocationGrandParent `json:"parents"`
}

type glintsLocationGrandParent struct {
	Name string `json:"name"`
}

type glintsSalary struct {
	MinAmount  int    `json:"minAmount"`
	MaxAmount  int    `json:"maxAmount"`
	SalaryMode string `json:"salaryMode"`
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

// ponytail: Jobstreet uses SEEK's GraphQL API. The endpoint and schema can change.
// Upgrade path: use browser automation to scrape if the API changes.
type jobstreetVars struct {
	Keywords   string `json:"keywords"`
	Page       int    `json:"page"`
	PageSize   int    `json:"pageSize"`
	Location   string `json:"location,omitempty"`
	CountryCode string `json:"countryCode,omitempty"`
}

func fetchJobstreetJobs(client *http.Client) []Job {
	var all []Job
	seen := map[string]bool{}

	for _, keyword := range keywords {
		for page := 1; page <= jobstreetMaxPages; page++ {
			jobs := fetchJobstreetPage(client, keyword, page)
			for _, j := range jobs {
				if seen[j.URL] {
					continue
				}
				seen[j.URL] = true
				all = append(all, j)
			}
			if len(jobs) < jobsPerRequest {
				break
			}
		}
	}

	return all
}

func fetchJobstreetPage(client *http.Client, keyword string, page int) []Job {
	// Jobstreet uses a REST search endpoint with URL params
	url := fmt.Sprintf("%s?keywords=%s&page=%d&pageSize=%d&countryCode=ID",
		jobstreetSearchURL,
		strings.ReplaceAll(keyword, " ", "+"),
		page,
		jobsPerRequest,
	)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; JobAlertBot/1.0)")

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

	return parseJobstreetResponse(body)
}

// parseJobstreetResponse tries multiple response formats.
// ponytail: Jobstreet API response format has changed in the past.
// Upgrade path: inspect the actual response and update the struct.
func parseJobstreetResponse(body []byte) []Job {
	// Try SEEK-style envelope
	type seekJob struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		Advertiser   struct {
			Description string `json:"description"`
		} `json:"advertiser"`
		Location     string `json:"location"`
		WorkType     string `json:"workType"`
		Salary       string `json:"salary"`
		ListingDate  string `json:"listingDate"`
	}

	type seekResponse struct {
		Data []seekJob `json:"data"`
	}

	var resp seekResponse
	if err := json.Unmarshal(body, &resp); err == nil && len(resp.Data) > 0 {
		jobs := make([]Job, 0, len(resp.Data))
		for _, j := range resp.Data {
			url := fmt.Sprintf("https://www.jobstreet.co.id/id/job/%s", j.ID)
			jobs = append(jobs, Job{
				Title:    j.Title,
				Company:  j.Advertiser.Description,
				Location: j.Location,
				URL:      url,
				Source:   "jobstreet",
				Salary:   j.Salary,
				Type:     j.WorkType,
				PostedAt: j.ListingDate,
			})
		}
		return jobs
	}

	// Try alternative format (graphql-style)
	type jsGraphQLJob struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Company struct {
			Name string `json:"name"`
		} `json:"company"`
		Location struct {
			Name string `json:"name"`
		} `json:"location"`
		WorkType    string `json:"workType"`
		SalaryLabel string `json:"salaryLabel"`
		ListingDate string `json:"listingDate"`
	}

	type jsGraphQLResponse struct {
		Data struct {
			SearchJobs struct {
				Jobs []jsGraphQLJob `json:"jobs"`
			} `json:"searchJobs"`
		} `json:"data"`
	}

	var gqlResp jsGraphQLResponse
	if err := json.Unmarshal(body, &gqlResp); err == nil && len(gqlResp.Data.SearchJobs.Jobs) > 0 {
		jobs := make([]Job, 0, len(gqlResp.Data.SearchJobs.Jobs))
		for _, j := range gqlResp.Data.SearchJobs.Jobs {
			url := fmt.Sprintf("https://www.jobstreet.co.id/id/job/%s", j.ID)
			jobs = append(jobs, Job{
				Title:    j.Title,
				Company:  j.Company.Name,
				Location: j.Location.Name,
				URL:      url,
				Source:   "jobstreet",
				Salary:   j.SalaryLabel,
				Type:     j.WorkType,
				PostedAt: j.ListingDate,
			})
		}
		return jobs
	}

	return nil
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
	if j.MinYearsExp > 3 {
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

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

func aiFilterJobs(client *http.Client, jobs []Job) []Job {
	if len(jobs) == 0 {
		return nil
	}

	jobsJSON, _ := json.Marshal(jobs)

	systemPrompt := fmt.Sprintf(`Kamu adalah filter lowongan kerja. Analisis setiap job listing JSON berikut dan RETURN HANYA array JSON job yang match dengan criteria:

TECH STACK (minimal 1 yang match):
%s

PENGALAMAN KERJA: 1-3 tahun (toleransi jika tidak disebutkan atau ≤3 tahun)

LOKASI: jabodetabek, bandung, surabaya, bali, batam, solo, salatiga, karawang, cikampek, cikarang.

Rules:
- Return HANYA JSON array dari job yang match, tidak boleh ada text lain.
- Pisahkan mana dari glints dan mana dari jobstreet (field "source").
- Kalau tidak ada yang match, return [].
- Jangan ubah struktur data job. Tetap gunakan field yang sama.
- Urutkan berdasarkan relevansi tech stack (terbanyak match duluan).
- Maksimal kembalikan 30 job.`, strings.Join(techStack, ", "))

	req := chatRequest{
		Model: "deepseek-v4-pro",
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: string(jobsJSON)},
		},
		MaxTokens: 4096,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return jobs // fallback: return unfiltered
	}

	httpReq, err := http.NewRequest(http.MethodPost, sumopodChatURL, bytes.NewReader(body))
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

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return jobs
	}

	if len(chatResp.Choices) == 0 {
		return jobs
	}

	content := chatResp.Choices[0].Message.Content
	content = cleanJSONResponse(content)

	var filtered []Job
	if err := json.Unmarshal([]byte(content), &filtered); err != nil {
		return jobs
	}

	return filtered
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
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "Markdown",
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

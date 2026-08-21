package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	kitalulusURL        = "https://kitalulus.com/lowongan"
	deallsURL           = "https://dealls.com/loker"
	sumopodURL          = "https://ai.sumopod.com/v1/responses"
	openAIResponsesURL  = "https://api.openai.com/v1/responses"
	googleGenerativeURL = "https://generativelanguage.googleapis.com/v1beta"
	telegramAPIURL      = "https://api.telegram.org"
	requestTimeout      = 30 * time.Second
	jobsPerSource       = 20
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
	HalalStatus string `json:"-"`
	HalalReason string `json:"-"`
}

func main() {
	keywordsFlag := flag.String("keywords", "", "comma-separated job titles")
	locationFlag := flag.String("location", "", "comma-separated locations")
	skillsFlag := flag.String("skills", "", "comma-separated skills")
	experienceFlag := flag.String("experience", "", "experience range, e.g. 1-3")
	halalFlag := flag.Bool("halal", false, "assess company business against halal criteria")
	dryRun := flag.Bool("dry-run", false, "print instead of sending")
	flag.Parse()

	keywords = splitAndTrim(*keywordsFlag)
	targetLocations = splitAndTrim(*locationFlag)
	techStack = splitAndTrim(*skillsFlag)
	maxYearsExp = parseMaxYears(*experienceFlag)

	client := &http.Client{Timeout: requestTimeout}
	fmt.Fprintln(os.Stderr, "job-alert: fetching")
	kitalulusJobs := fetchKitalulusJobs(client, keywords)
	deallsJobs := fetchDeallsJobs(client, keywords)
	fmt.Fprintf(os.Stderr, "job-alert: fetched kitalulus=%d dealls=%d\n", len(kitalulusJobs), len(deallsJobs))

	kitalulusFinal := filterByKeywordOrSkill(filterJobs(kitalulusJobs), keywords, techStack)
	deallsFinal := filterByKeywordOrSkill(filterJobs(deallsJobs), keywords, techStack)
	fmt.Fprintf(os.Stderr, "job-alert: matched kitalulus=%d dealls=%d\n", len(kitalulusFinal), len(deallsFinal))
	sortJobsByRelevance(kitalulusFinal)
	sortJobsByRelevance(deallsFinal)
	kitalulusFinal = limitJobs(kitalulusFinal, jobsPerSource)
	deallsFinal = limitJobs(deallsFinal, jobsPerSource)
	if *halalFlag {
		assessHalalCompanies(client, kitalulusFinal, deallsFinal)
	}

	greeting := "Selamat pagi! ☀️ Berikut update lowongan kerja terbaru hari ini:"
	if *keywordsFlag != "" {
		greeting = "Berikut hasil pencarian lowongan kerja:"
	}
	message := formatMessage(greeting, kitalulusFinal, deallsFinal)
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

// parseMaxYears extracts the upper bound from an experience flag.
// Accepts "3" or "1-3"; the last number is the max.
func parseMaxYears(s string) int {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, "-")
	last := strings.TrimSpace(parts[len(parts)-1])
	var max int
	fmt.Sscanf(last, "%d", &max)
	return max
}

// --- Shared fetch helpers ---

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

var (
	nextDataRe       = regexp.MustCompile(`<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`)
	kitalulusCardRe  = regexp.MustCompile(`<a[^>]*href="/lowongan/detail/([^"]+)"[^>]*>(.*?)</a>`)
	kitalulusTitleRe = regexp.MustCompile(`<h3[^>]*>(.*?)</h3>`)
	kitalulusPRe     = regexp.MustCompile(`<p[^>]*>(.*?)</p>`)
	htmlTagRe        = regexp.MustCompile(`<[^>]*>`)
)

func fetchHTML(client *http.Client, url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "id-ID,id;q=0.9")
	req.Header.Set("User-Agent", browserUA)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
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

func buildSearchURL(base, parameter string, keywords []string) string {
	query := strings.Join(keywords, " ")
	if query == "" {
		return base
	}
	return base + "?" + url.Values{parameter: []string{query}}.Encode()
}

// --- Kitalulus Fetcher ---
// Kitalulus is SSR; cards are <a href="/lowongan/detail/<slug>"> with <h3> title.

func fetchKitalulusJobs(client *http.Client, searchKeywords []string) []Job {
	var (
		mu   sync.Mutex
		all  []Job
		seen = map[string]bool{}
	)
	var wg sync.WaitGroup
	for _, kw := range searchKeywords {
		wg.Add(1)
		go func(keyword string) {
			defer wg.Done()
			searchURL := buildSearchURL(kitalulusURL, "keyword", []string{keyword})
			html, err := fetchHTML(client, searchURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "job-alert: kitalulus fetch (%s): %v\n", keyword, err)
				return
			}
			for _, j := range parseKitalulusHTML(html) {
				mu.Lock()
				if !seen[j.URL] {
					seen[j.URL] = true
					all = append(all, j)
				}
				mu.Unlock()
			}
		}(kw)
	}
	wg.Wait()
	return all
}

func parseKitalulusHTML(html string) []Job {
	matches := kitalulusCardRe.FindAllStringSubmatch(html, -1)
	jobs := make([]Job, 0, len(matches))
	for _, m := range matches {
		slug, card := m[1], m[2]
		title := ""
		if tm := kitalulusTitleRe.FindStringSubmatch(card); len(tm) > 1 {
			title = decodeHTML(stripTags(tm[1]))
		}
		var details []string
		for _, pm := range kitalulusPRe.FindAllStringSubmatch(card, -1) {
			txt := decodeHTML(stripTags(pm[1]))
			if txt == "" || txt == "Dipromosikan" {
				continue
			}
			details = append(details, txt)
		}
		company := ""
		location := ""
		if len(details) > 0 {
			company = strings.TrimSpace(strings.SplitN(details[0], " - ", 2)[0])
		}
		if len(details) > 1 {
			location = details[1]
		}
		jobs = append(jobs, Job{
			Title:    title,
			Company:  company,
			Location: location,
			URL:      "https://kitalulus.com/lowongan/detail/" + slug,
			Source:   "kitalulus",
		})
	}
	return jobs
}

// --- Dealls Fetcher ---
// Dealls is Next.js SSR; jobs are in __NEXT_DATA__.props.pageProps.dehydratedState.

type deallsJob struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Role    string `json:"role"`
	Company struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"company"`
	City struct {
		Name string `json:"name"`
	} `json:"city"`
	SalaryRange *struct {
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"salaryRange"`
	Skills []struct {
		Name string `json:"name"`
	} `json:"skills"`
	WorkplaceType   string   `json:"workplaceType"`
	EmploymentTypes []string `json:"employmentTypes"`
	PublishedAt     string   `json:"publishedAt"`
}

type deallsNextData struct {
	Props struct {
		PageProps struct {
			DehydratedState struct {
				Queries []struct {
					State struct {
						Data struct {
							Pages []struct {
								Docs []deallsJob `json:"docs"`
							} `json:"pages"`
						} `json:"data"`
					} `json:"state"`
				} `json:"queries"`
			} `json:"dehydratedState"`
		} `json:"pageProps"`
	} `json:"props"`
}

func fetchDeallsJobs(client *http.Client, searchKeywords []string) []Job {
	searchURL := buildSearchURL(deallsURL, "search", searchKeywords)
	html, err := fetchHTML(client, searchURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "job-alert: dealls fetch: %v\n", err)
		return nil
	}
	return parseDeallsHTML(html)
}

func parseDeallsHTML(html string) []Job {
	m := nextDataRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil
	}
	var data deallsNextData
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		fmt.Fprintf(os.Stderr, "job-alert: dealls parse: %v\n", err)
		return nil
	}
	var jobs []Job
	for _, q := range data.Props.PageProps.DehydratedState.Queries {
		for _, p := range q.State.Data.Pages {
			for _, j := range p.Docs {
				skills := make([]string, 0, len(j.Skills))
				for _, s := range j.Skills {
					skills = append(skills, s.Name)
				}
				u := fmt.Sprintf("https://dealls.com/loker/%s~%s", j.Slug, j.Company.Slug)
				jobs = append(jobs, Job{
					Title:    j.Role,
					Company:  j.Company.Name,
					Location: j.City.Name,
					URL:      u,
					Source:   "dealls",
					Salary:   formatDeallsSalary(j.SalaryRange),
					Type:     strings.Join(j.EmploymentTypes, ", "),
					Skills:   strings.Join(skills, ", "),
					PostedAt: j.PublishedAt,
				})
			}
		}
	}
	return jobs
}

func formatDeallsSalary(r *struct {
	Start int `json:"start"`
	End   int `json:"end"`
}) string {
	if r == nil {
		return ""
	}
	if r.Start == 0 && r.End == 0 {
		return ""
	}
	if r.Start == r.End {
		return fmt.Sprintf("Rp %s jt", formatMillions(r.Start))
	}
	return fmt.Sprintf("Rp %s - %s jt", formatMillions(r.Start), formatMillions(r.End))
}

func formatMillions(amount int) string {
	m := float64(amount) / 1_000_000
	if m == float64(int(m)) {
		return fmt.Sprintf("%.0f", m)
	}
	return fmt.Sprintf("%.1f", m)
}

// --- Location Filtering ---

func matchesLocation(location string) bool {
	lower := normalizeLocation(location)
	for _, loc := range targetLocations {
		if strings.Contains(lower, normalizeLocation(loc)) {
			return true
		}
	}
	return false
}

func normalizeLocation(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "south jakarta", "jakarta selatan")
	s = strings.ReplaceAll(s, "central jakarta", "jakarta pusat")
	s = strings.ReplaceAll(s, "north jakarta", "jakarta utara")
	s = strings.ReplaceAll(s, "west jakarta", "jakarta barat")
	s = strings.ReplaceAll(s, "east jakarta", "jakarta timur")
	s = strings.ReplaceAll(s, "jakarta raya", "jakarta")
	return s
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
		if (len(targetLocations) == 0 || matchesLocation(j.Location)) && matchesExperience(j) {
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

// --- Deterministic Relevance Filtering ---

func filterByKeywordOrSkill(jobs []Job, searchKeywords, stack []string) []Job {
	var out []Job
	for _, j := range jobs {
		title := strings.ToLower(j.Title)

		// A supplied position must match the job title, not company or skills.
		if len(searchKeywords) > 0 && !matchesAnyTerm(title, searchKeywords) {
			continue
		}
		// Some sources do not expose skills. Do not turn missing source data into
		// a negative match; when skills are available, require one requested skill.
		if len(stack) > 0 && strings.TrimSpace(j.Skills) != "" && !matchesAnyTerm(strings.ToLower(j.Skills), stack) {
			continue
		}
		out = append(out, j)
	}
	return out
}

func matchesAnyTerm(text string, terms []string) bool {
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term != "" && strings.Contains(text, term) {
			return true
		}
		for _, word := range strings.Fields(term) {
			if len(word) > 2 && strings.Contains(text, word) {
				return true
			}
		}
	}
	return false
}

// --- AI Company Assessment ---

// Sumopod and OpenAI expose the Responses API (not chat completions).
// Request: {model, instructions, input}; response: output[].content[].text.

type responsesRequest struct {
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
	Input        string `json:"input"`
}

type responsesResponse struct {
	Output []struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

type geminiRequest struct {
	SystemInstruction geminiContent   `json:"system_instruction"`
	Contents          []geminiContent `json:"contents"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
}

const (
	halalStatusHalal       = "halal"
	halalStatusNotHalal    = "tidak_halal"
	halalStatusNeedsReview = "perlu_riset"
)

type companyAssessmentInput struct {
	Company string   `json:"company"`
	Roles   []string `json:"roles"`
}

type companyAssessment struct {
	Company string `json:"company"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
}

const halalAssessmentInstructions = `Nilai model bisnis setiap perusahaan berdasarkan informasi publik yang benar-benar kamu ketahui.

Kriteria halal:
1. Bukan produsen atau penjual utama barang/jasa yang haram.
2. Model bisnis utama tidak berkaitan dengan riba.
3. Bukan bank, asuransi, perusahaan pinjaman online, atau bisnis pembiayaan berbunga.

Return HANYA array JSON dengan satu object per perusahaan: {"company":"nama asli","status":"halal|tidak_halal|perlu_riset","reason":"alasan singkat"}.
- Gunakan halal hanya jika model bisnis utamanya jelas memenuhi ketiga kriteria.
- Gunakan tidak_halal jika jelas melanggar, dan sebutkan alasan spesifik seperti Bank, Asuransi, Pinjaman online, Riba, atau Barang haram.
- Gunakan perlu_riset jika informasi tidak cukup atau identitas perusahaan ambigu. Jangan menebak.
- Jangan berikan fatwa, markdown, atau teks di luar JSON.`

func assessHalalCompanies(client *http.Client, groups ...[]Job) {
	companies := make([]companyAssessmentInput, 0)
	companyIndex := make(map[string]int)
	for _, jobs := range groups {
		for i := range jobs {
			jobs[i].HalalStatus = halalStatusNeedsReview
			key := normalizeCompany(jobs[i].Company)
			if key == "" {
				continue
			}
			index, exists := companyIndex[key]
			if !exists {
				index = len(companies)
				companyIndex[key] = index
				companies = append(companies, companyAssessmentInput{Company: jobs[i].Company})
			}
			companies[index].Roles = append(companies[index].Roles, jobs[i].Title)
		}
	}
	if len(companies) == 0 {
		return
	}

	model := strings.TrimSpace(os.Getenv("AI_MODEL"))
	if model == "" {
		fmt.Fprintln(os.Stderr, "job-alert: halal assessment skipped: AI_MODEL not set")
		return
	}
	input, err := json.Marshal(companies)
	if err != nil {
		return
	}
	assessmentClient := *client
	assessmentClient.Timeout = 120 * time.Second
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("AI_PROVIDER")))
	if provider == "" {
		provider = "sumopod"
	}
	responseText, err := requestHalalAssessment(&assessmentClient, provider, model, string(input))
	if err != nil {
		fmt.Fprintf(os.Stderr, "job-alert: halal assessment failed: %v\n", err)
		return
	}
	var assessments []companyAssessment
	if err := json.Unmarshal([]byte(cleanJSONResponse(responseText)), &assessments); err != nil {
		fmt.Fprintf(os.Stderr, "job-alert: halal assessment parse: %v\n", err)
		return
	}
	applyCompanyAssessments(groups, assessments)
}

func requestHalalAssessment(client *http.Client, provider, model, input string) (string, error) {
	switch provider {
	case "sumopod":
		return requestResponsesAssessment(client, getenv("SUMOPOD_RESPONSES_URL", sumopodURL), os.Getenv("SUMOPOD_API_KEY"), model, input)
	case "openai":
		return requestResponsesAssessment(client, getenv("OPENAI_RESPONSES_URL", openAIResponsesURL), os.Getenv("OPENAI_API_KEY"), model, input)
	case "google":
		return requestGeminiAssessment(client, model, input)
	default:
		return "", fmt.Errorf("unsupported AI_PROVIDER %q", provider)
	}
}

func requestResponsesAssessment(client *http.Client, endpoint, apiKey, model, input string) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", fmt.Errorf("API key not set")
	}
	body, err := json.Marshal(responsesRequest{Model: model, Instructions: halalAssessmentInstructions, Input: input})
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("provider returned status %d", resp.StatusCode)
	}
	response, err := decodeResponsesResponse(resp)
	if err != nil {
		return "", err
	}
	return extractResponsesText(response), nil
}

func decodeResponsesResponse(resp *http.Response) (responsesResponse, error) {
	var response responsesResponse
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return response, json.NewDecoder(resp.Body).Decode(&response)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 8<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event struct {
			Type     string            `json:"type"`
			Response responsesResponse `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &event); err == nil && event.Type == "response.completed" {
			return event.Response, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return response, err
	}
	return response, fmt.Errorf("response.completed event not found")
}

func requestGeminiAssessment(client *http.Client, model, input string) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("GOOGLE_API_KEY"))
	if apiKey == "" {
		return "", fmt.Errorf("API key not set")
	}
	body, err := json.Marshal(geminiRequest{
		SystemInstruction: geminiContent{Parts: []geminiPart{{Text: halalAssessmentInstructions}}},
		Contents:          []geminiContent{{Role: "user", Parts: []geminiPart{{Text: input}}}},
	})
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(getenv("GOOGLE_GENERATIVE_URL", googleGenerativeURL), "/") + "/models/" + url.PathEscape(model) + ":generateContent"
	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Goog-Api-Key", apiKey)
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("provider returned status %d", resp.StatusCode)
	}
	var response geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				return part.Text, nil
			}
		}
	}
	return "", nil
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func applyCompanyAssessments(groups [][]Job, assessments []companyAssessment) {
	byCompany := make(map[string]companyAssessment, len(assessments))
	for _, assessment := range assessments {
		if assessment.Status != halalStatusHalal && assessment.Status != halalStatusNotHalal && assessment.Status != halalStatusNeedsReview {
			continue
		}
		byCompany[normalizeCompany(assessment.Company)] = assessment
	}
	for _, jobs := range groups {
		for i := range jobs {
			if assessment, ok := byCompany[normalizeCompany(jobs[i].Company)]; ok {
				jobs[i].HalalStatus = assessment.Status
				jobs[i].HalalReason = strings.Join(strings.Fields(assessment.Reason), " ")
			}
		}
	}
}

func normalizeCompany(company string) string {
	return strings.ToLower(strings.TrimSpace(company))
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
	return strings.TrimSpace(s)
}

// --- Message Formatter ---

func formatMessage(greeting string, kitalulusJobs, deallsJobs []Job) string {
	var b strings.Builder

	b.WriteString(greeting)
	b.WriteString("\n\nDaftar Job Terbaru:\n\n")

	writeSection(&b, "A. Kitalulus", kitalulusJobs)
	writeSection(&b, "B. Dealls", deallsJobs)

	b.WriteString("\n— Dikirim otomatis oleh Job Alert Bot")

	return b.String()
}

func writeSection(b *strings.Builder, header string, jobs []Job) {
	b.WriteString(header)
	b.WriteString("\n\n")
	if len(jobs) == 0 {
		b.WriteString("Tidak ada lowongan baru yang sesuai kriteria.\n\n")
		return
	}
	for i, j := range jobs {
		writeJobEntry(b, i+1, j)
	}
	b.WriteString("\n")
}

func writeJobEntry(b *strings.Builder, num int, j Job) {
	b.WriteString(fmt.Sprintf("%d. **%s**\n", num, j.Title))
	if j.Company != "" {
		b.WriteString(fmt.Sprintf("   🏢 %s%s\n", j.Company, halalLabel(j)))
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

func halalLabel(job Job) string {
	switch job.HalalStatus {
	case halalStatusHalal:
		return " (Halal)"
	case halalStatusNotHalal:
		if job.HalalReason != "" {
			return " (Tidak Halal — " + job.HalalReason + ")"
		}
		return " (Tidak Halal)"
	case halalStatusNeedsReview:
		return " (Perlu Riset)"
	default:
		return ""
	}
}

// --- Telegram Sender ---

func sendTelegram(client *http.Client, message string) {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_USER_ID")

	if botToken == "" || chatID == "" {
		fmt.Fprintln(os.Stderr, "job-alert: Telegram credentials missing, printing to stdout")
		fmt.Println(message)
		return
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", telegramAPIURL, botToken)
	for _, chunk := range splitTelegramMessage(message, 4000) {
		payload := map[string]string{
			"chat_id":                  chatID,
			"text":                     chunk,
			"parse_mode":               "Markdown",
			"disable_web_page_preview": "true",
		}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "job-alert: Telegram send failed: %v\n", err)
			return
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "job-alert: Telegram API error (%d): %s\n", resp.StatusCode, string(respBody))
			return
		}
	}
	fmt.Fprintln(os.Stderr, "job-alert: message sent to Telegram")
}

func splitTelegramMessage(message string, max int) []string {
	var chunks []string
	for len(message) > max {
		cut := strings.LastIndex(message[:max], "\n\n")
		if cut < max/2 {
			cut = strings.LastIndex(message[:max], "\n")
		}
		if cut < 1 {
			cut = max
		}
		chunks = append(chunks, message[:cut])
		message = message[cut:]
	}
	if message != "" {
		chunks = append(chunks, message)
	}
	return chunks
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

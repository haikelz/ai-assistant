package main

import (
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
	kalibrrURL     = "https://www.kalibrr.com/id-ID/job-board/te/software"
	kitalulusURL   = "https://kitalulus.com/lowongan"
	deallsURL      = "https://dealls.com/loker"
	sumopodURL     = "https://ai.sumopod.com/v1/responses"
	telegramAPIURL = "https://api.telegram.org"
	requestTimeout = 30 * time.Second
	jobsPerSource  = 20
	maxPages       = 2
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
	skillsFlag := flag.String("skills", "", "comma-separated tech stack for filtering (default: built-in list)")
	experienceFlag := flag.String("experience", "", "max years of experience, e.g. 3 or 1-3 (default: 3)")
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
		maxYearsExp = parseMaxYears(*experienceFlag)
	}

	client := &http.Client{Timeout: requestTimeout}

	fmt.Fprintln(os.Stderr, "job-alert: fetching")
	kalibrrJobs := fetchKalibrrJobs(client)
	kitalulusJobs := fetchKitalulusJobs(client, keywords)
	deallsJobs := fetchDeallsJobs(client)
	fmt.Fprintf(os.Stderr, "job-alert: fetched kalibrr=%d kitalulus=%d dealls=%d\n", len(kalibrrJobs), len(kitalulusJobs), len(deallsJobs))

	kalibrrFiltered := filterJobs(kalibrrJobs)
	kitalulusFiltered := filterJobs(kitalulusJobs)
	deallsFiltered := filterJobs(deallsJobs)

	kalibrrFinal := aiFilterJobs(client, kalibrrFiltered, techStack)
	kitalulusFinal := aiFilterJobs(client, kitalulusFiltered, techStack)
	deallsFinal := aiFilterJobs(client, deallsFiltered, techStack)

	sortJobsByRelevance(kalibrrFinal)
	sortJobsByRelevance(kitalulusFinal)
	sortJobsByRelevance(deallsFinal)
	kalibrrFinal = limitJobs(kalibrrFinal, jobsPerSource)
	kitalulusFinal = limitJobs(kitalulusFinal, jobsPerSource)
	deallsFinal = limitJobs(deallsFinal, jobsPerSource)

	greeting := "Selamat pagi! ☀️ Berikut update lowongan kerja terbaru hari ini:"
	if *keywordsFlag != "" {
		greeting = "Berikut hasil pencarian lowongan kerja:"
	}
	message := formatMessage(greeting, kalibrrFinal, kitalulusFinal, deallsFinal)

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
	liRe             = regexp.MustCompile(`<li[^>]*>(.*?)</li>`)
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

func extractListItems(s string) string {
	items := liRe.FindAllStringSubmatch(s, -1)
	parts := make([]string, 0, len(items))
	for _, m := range items {
		if txt := decodeHTML(stripTags(m[1])); txt != "" {
			parts = append(parts, txt)
		}
	}
	return strings.Join(parts, ", ")
}

// --- Kalibrr Fetcher ---
// Kalibrr is Next.js SSR; jobs are embedded in __NEXT_DATA__ JSON.

type kalibrrJob struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	CompanyName string `json:"companyName"`
	Company     struct {
		Code string `json:"code"`
	} `json:"company"`
	Slug           string `json:"slug"`
	Function       string `json:"function"`
	CreatedAt      string `json:"createdAt"`
	BaseSalary     *int   `json:"baseSalary"`
	MaximumSalary  *int   `json:"maximumSalary"`
	SalaryCurrency string `json:"salaryCurrency"`
	GoogleLocation struct {
		AddressComponents struct {
			City   string `json:"city"`
			Region string `json:"region"`
		} `json:"addressComponents"`
	} `json:"googleLocation"`
	Qualifications string `json:"qualifications"`
}

type kalibrrNextData struct {
	Props struct {
		PageProps struct {
			Jobs []kalibrrJob `json:"jobs"`
		} `json:"pageProps"`
	} `json:"props"`
}

func fetchKalibrrJobs(client *http.Client) []Job {
	var all []Job
	for page := 1; page <= maxPages; page++ {
		url := fmt.Sprintf("%s?page=%d", kalibrrURL, page)
		html, err := fetchHTML(client, url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "job-alert: kalibrr fetch: %v\n", err)
			break
		}
		jobs := parseKalibrrHTML(html)
		if len(jobs) == 0 {
			break
		}
		all = append(all, jobs...)
	}
	return all
}

func parseKalibrrHTML(html string) []Job {
	m := nextDataRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil
	}
	var data kalibrrNextData
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		fmt.Fprintf(os.Stderr, "job-alert: kalibrr parse: %v\n", err)
		return nil
	}
	jobs := make([]Job, 0, len(data.Props.PageProps.Jobs))
	for _, j := range data.Props.PageProps.Jobs {
		loc := strings.TrimSpace(j.GoogleLocation.AddressComponents.City)
		if r := strings.TrimSpace(j.GoogleLocation.AddressComponents.Region); r != "" && r != loc {
			loc += ", " + r
		}
		u := fmt.Sprintf("https://www.kalibrr.com/id-ID/c/%s/jobs/%d/%s", j.Company.Code, j.ID, j.Slug)
		jobs = append(jobs, Job{
			Title:    j.Name,
			Company:  j.CompanyName,
			Location: loc,
			URL:      u,
			Source:   "kalibrr",
			Salary:   formatSalaryRange(derefInt(j.BaseSalary), derefInt(j.MaximumSalary)),
			Type:     j.Function,
			Skills:   extractListItems(j.Qualifications),
			PostedAt: j.CreatedAt,
		})
	}
	return jobs
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
			url := fmt.Sprintf("%s?q=%s", kitalulusURL, url.QueryEscape(keyword))
			html, err := fetchHTML(client, url)
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
		company := ""
		location := ""
		firstP := true
		for _, pm := range kitalulusPRe.FindAllStringSubmatch(card, -1) {
			txt := decodeHTML(stripTags(pm[1]))
			if txt == "" || txt == "Dipromosikan" {
				continue
			}
			if isLocation(txt) {
				location = txt
				continue
			}
			if firstP {
				company = strings.TrimSpace(strings.SplitN(txt, " - ", 2)[0])
				firstP = false
			}
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

func isLocation(s string) bool {
	lower := strings.ToLower(s)
	for _, loc := range targetLocations {
		if strings.Contains(lower, loc) {
			return true
		}
	}
	return false
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

func fetchDeallsJobs(client *http.Client) []Job {
	html, err := fetchHTML(client, deallsURL)
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

func formatSalaryRange(min, max int) string {
	if min == 0 && max == 0 {
		return ""
	}
	if min != 0 && max != 0 && min != max {
		return fmt.Sprintf("Rp %s - %s jt", formatMillions(min), formatMillions(max))
	}
	v := min
	if v == 0 {
		v = max
	}
	return fmt.Sprintf("Rp %s jt", formatMillions(v))
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

type responsesResponse struct {
	Output []struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

func aiFilterJobs(client *http.Client, jobs []Job, stack []string) []Job {
	if len(jobs) == 0 {
		return nil
	}

	jobsJSON, _ := json.Marshal(jobs)

	instructions := fmt.Sprintf(`Kamu filter lowongan kerja. Dari JSON di bawah, return HANYA array JSON job yang match kriteria:

TECH STACK (minimal 1 match):
%s

PENGALAMAN KERJA: 1-3 tahun (toleransi jika tidak disebutkan atau <=3).

Rules:
- Return HANYA JSON array, tanpa teks lain, tanpa markdown.
- Jangan ubah struktur object job, kembalikan object aslinya persis.
- Kalau tidak ada yang match, return [].
- Maksimal 30 job.`, strings.Join(stack, ", "))

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
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+os.Getenv("SUMOPOD_API_KEY"))

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

	content := cleanJSONResponse(extractResponsesText(rresp))
	var filtered []Job
	if err := json.Unmarshal([]byte(content), &filtered); err != nil || len(filtered) == 0 {
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

func formatMessage(greeting string, kalibrrJobs, kitalulusJobs, deallsJobs []Job) string {
	var b strings.Builder

	b.WriteString(greeting)
	b.WriteString("\n\nDaftar Job Terbaru:\n\n")

	writeSection(&b, "A. Kalibrr", kalibrrJobs)
	writeSection(&b, "B. Kitalulus", kitalulusJobs)
	writeSection(&b, "C. Dealls", deallsJobs)

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
		fmt.Fprintln(os.Stderr, "job-alert: TELEGRAM_BOT_TOKEN or TELEGRAM_USER_ID not set, printing to stdout")
		fmt.Println(message)
		return
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", telegramAPIURL, botToken)

	text := message
	if len(message) > 4000 {
		text = message[:4000] + "\n\n— (dipotong karena terlalu panjang)"
	}

	payload := map[string]string{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "Markdown",
		"disable_web_page_preview": "true",
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "job-alert: failed to send telegram message: %v\n", err)
		fmt.Println(message)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "job-alert: telegram API error (%d): %s\n", resp.StatusCode, string(respBody))
		fmt.Println(message)
		return
	}
	fmt.Fprintln(os.Stderr, "job-alert: message sent to Telegram")
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

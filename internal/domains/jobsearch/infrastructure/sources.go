package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"ai-assistant/internal/domains/jobsearch/domain"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120 Safari/537.36"

type Kitalulus struct {
	client *http.Client
	base   string
}

func NewKitalulus(client *http.Client, baseURL string) *Kitalulus {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = "https://kitalulus.com/lowongan"
	}
	return &Kitalulus{client, strings.TrimRight(baseURL, "/")}
}
func (*Kitalulus) Name() string { return "kitalulus" }

var cardRE = regexp.MustCompile(`<a[^>]*href="/lowongan/detail/([^"]+)"[^>]*>(.*?)</a>`)
var titleRE = regexp.MustCompile(`<h3[^>]*>(.*?)</h3>`)
var pRE = regexp.MustCompile(`<p[^>]*>(.*?)</p>`)
var tagRE = regexp.MustCompile(`<[^>]*>`)

func (k *Kitalulus) Fetch(ctx context.Context, c domain.Criteria) ([]domain.Job, error) {
	terms := c.Positions
	if len(terms) == 0 {
		terms = []string{""}
	}
	var mu sync.Mutex
	seen := map[string]bool{}
	var jobs []domain.Job
	var first error
	succeeded := 0
	var wg sync.WaitGroup
	for _, term := range terms {
		wg.Add(1)
		go func(term string) {
			defer wg.Done()
			u := k.base
			if term != "" {
				u += "?" + url.Values{"keyword": {term}}.Encode()
			}
			body, err := fetch(ctx, k.client, u)
			if err != nil {
				mu.Lock()
				if first == nil {
					first = err
				}
				mu.Unlock()
				return
			}
			for _, j := range parseKitalulus(body, k.base) {
				mu.Lock()
				if !seen[j.URL] {
					seen[j.URL] = true
					jobs = append(jobs, j)
				}
				mu.Unlock()
			}
			mu.Lock()
			succeeded++
			mu.Unlock()
		}(term)
	}
	wg.Wait()
	if succeeded > 0 {
		first = nil
	}
	return jobs, first
}
func parseKitalulus(body, base string) []domain.Job {
	var out []domain.Job
	origin := "https://kitalulus.com"
	if u, e := url.Parse(base); e == nil {
		origin = u.Scheme + "://" + u.Host
	}
	for _, m := range cardRE.FindAllStringSubmatch(body, -1) {
		title := ""
		if x := titleRE.FindStringSubmatch(m[2]); len(x) > 1 {
			title = clean(x[1])
		}
		var d []string
		for _, x := range pRE.FindAllStringSubmatch(m[2], -1) {
			v := clean(x[1])
			if v != "" && v != "Dipromosikan" {
				d = append(d, v)
			}
		}
		j := domain.Job{Title: title, URL: origin + "/lowongan/detail/" + m[1], Source: "kitalulus"}
		if len(d) > 0 {
			j.Company = strings.TrimSpace(strings.SplitN(d[0], " - ", 2)[0])
		}
		if len(d) > 1 {
			j.Location = d[1]
		}
		out = append(out, j)
	}
	return out
}
func clean(s string) string {
	return strings.TrimSpace(html.UnescapeString(tagRE.ReplaceAllString(s, "")))
}

type Dealls struct {
	client *http.Client
	base   string
}

func NewDealls(client *http.Client, baseURL string) *Dealls {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = "https://dealls.com/loker"
	}
	return &Dealls{client, strings.TrimRight(baseURL, "/")}
}
func (*Dealls) Name() string { return "dealls" }

var nextRE = regexp.MustCompile(`<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`)

type deallsJob struct {
	Slug, Role      string
	Company         struct{ Name, Slug string }
	City            struct{ Name string }
	SalaryRange     *struct{ Start, End int }
	Skills          []struct{ Name string }
	EmploymentTypes []string
	PublishedAt     string
}
type nextData struct {
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

func (d *Dealls) Fetch(ctx context.Context, c domain.Criteria) ([]domain.Job, error) {
	u := d.base
	if len(c.Positions) > 0 {
		u += "?" + url.Values{"search": {strings.Join(c.Positions, " ")}}.Encode()
	}
	body, err := fetch(ctx, d.client, u)
	if err != nil {
		return nil, err
	}
	m := nextRE.FindStringSubmatch(body)
	if len(m) < 2 {
		return nil, nil
	}
	var data nextData
	if err = json.Unmarshal([]byte(m[1]), &data); err != nil {
		return nil, err
	}
	var out []domain.Job
	origin := "https://dealls.com"
	if x, e := url.Parse(d.base); e == nil {
		origin = x.Scheme + "://" + x.Host
	}
	for _, q := range data.Props.PageProps.DehydratedState.Queries {
		for _, p := range q.State.Data.Pages {
			for _, j := range p.Docs {
				var skills []string
				for _, s := range j.Skills {
					skills = append(skills, s.Name)
				}
				out = append(out, domain.Job{Title: j.Role, Company: j.Company.Name, Location: j.City.Name, URL: fmt.Sprintf("%s/loker/%s~%s", origin, j.Slug, j.Company.Slug), Source: "dealls", Salary: salary(j.SalaryRange), Type: strings.Join(j.EmploymentTypes, ", "), Skills: strings.Join(skills, ", "), PostedAt: j.PublishedAt})
			}
		}
	}
	return out, nil
}
func salary(r *struct{ Start, End int }) string {
	if r == nil || (r.Start == 0 && r.End == 0) {
		return ""
	}
	f := func(n int) string {
		v := float64(n) / 1e6
		if v == float64(int(v)) {
			return fmt.Sprintf("%.0f", v)
		}
		return fmt.Sprintf("%.1f", v)
	}
	if r.Start == r.End {
		return "Rp " + f(r.Start) + " jt"
	}
	return "Rp " + f(r.Start) + " - " + f(r.End) + " jt"
}
func fetch(ctx context.Context, c *http.Client, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

package infrastructure

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type Config struct{ Provider, Model, SumopodAPIKey, OpenAIAPIKey, GoogleAPIKey, SumopodURL, OpenAIURL, GoogleURL string }
type AIAssessor struct {
	client *http.Client
	config Config
}

func NewAIAssessor(client *http.Client, c Config) *AIAssessor {
	if client == nil {
		client = http.DefaultClient
	}
	return &AIAssessor{client, c}
}

const instructions = `Nilai model bisnis setiap perusahaan berdasarkan informasi publik yang benar-benar kamu ketahui. Return HANYA array JSON: {"company":"nama asli","status":"halal|tidak_halal|perlu_riset","reason":"alasan singkat"}. Gunakan tidak_halal untuk bank, asuransi, pinjaman berbunga, riba, atau barang haram; perlu_riset jika ambigu. Jangan berikan fatwa atau markdown.`

type assessment struct{ Company, Status, Reason string }

func (a *AIAssessor) Assess(ctx context.Context, jobs []domain.Job) ([]domain.Job, error) {
	out := append([]domain.Job(nil), jobs...)
	companies := map[string]struct {
		Company string
		Roles   []string
	}{}
	for i, j := range out {
		out[i].HalalStatus = domain.HalalStatusNeedsReview
		k := strings.ToLower(strings.TrimSpace(j.Company))
		v := companies[k]
		v.Company = j.Company
		v.Roles = append(v.Roles, j.Title)
		companies[k] = v
	}
	if len(companies) == 0 || strings.TrimSpace(a.config.Model) == "" {
		return out, nil
	}
	companyList := make([]struct {
		Company string   `json:"company"`
		Roles   []string `json:"roles"`
	}, 0, len(companies))
	for _, company := range companies {
		companyList = append(companyList, struct {
			Company string   `json:"company"`
			Roles   []string `json:"roles"`
		}{company.Company, company.Roles})
	}
	input, _ := json.Marshal(companyList)
	text, err := a.request(ctx, string(input))
	if err != nil {
		return out, err
	}
	var as []assessment
	if err = json.Unmarshal([]byte(cleanJSON(text)), &as); err != nil {
		return out, err
	}
	by := map[string]assessment{}
	for _, v := range as {
		if v.Status == domain.HalalStatusHalal || v.Status == domain.HalalStatusNotHalal || v.Status == domain.HalalStatusNeedsReview {
			by[strings.ToLower(strings.TrimSpace(v.Company))] = v
		}
	}
	for i := range out {
		if v, ok := by[strings.ToLower(strings.TrimSpace(out[i].Company))]; ok {
			out[i].HalalStatus = v.Status
			out[i].HalalReason = strings.Join(strings.Fields(v.Reason), " ")
		}
	}
	return out, nil
}
func (a *AIAssessor) request(ctx context.Context, input string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(a.config.Provider))
	if p == "" {
		p = "sumopod"
	}
	if p == "google" {
		if strings.TrimSpace(a.config.GoogleAPIKey) == "" {
			return "", fmt.Errorf("Google API key not set")
		}
		base := a.config.GoogleURL
		if base == "" {
			base = "https://generativelanguage.googleapis.com/v1beta"
		}
		body, _ := json.Marshal(map[string]any{"system_instruction": map[string]any{"parts": []map[string]string{{"text": instructions}}}, "contents": []any{map[string]any{"role": "user", "parts": []map[string]string{{"text": input}}}}})
		return a.doGemini(ctx, strings.TrimRight(base, "/")+"/models/"+url.PathEscape(a.config.Model)+":generateContent", body)
	}
	endpoint, key := a.config.SumopodURL, a.config.SumopodAPIKey
	if p == "openai" {
		endpoint, key = a.config.OpenAIURL, a.config.OpenAIAPIKey
	}
	if p != "sumopod" && p != "openai" {
		return "", fmt.Errorf("unsupported AI provider %q", p)
	}
	if endpoint == "" {
		if p == "openai" {
			endpoint = "https://api.openai.com/v1/responses"
		} else {
			endpoint = "https://ai.sumopod.com/v1/responses"
		}
	}
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("%s API key not set", p)
	}
	body, _ := json.Marshal(map[string]string{"model": a.config.Model, "instructions": instructions, "input": input})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("provider returned status %d", resp.StatusCode)
	}
	return responsesText(resp)
}
func (a *AIAssessor) doGemini(ctx context.Context, endpoint string, body []byte) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("X-Goog-Api-Key", a.config.GoogleAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("provider returned status %d", resp.StatusCode)
	}
	var r struct {
		Candidates []struct {
			Content struct{ Parts []struct{ Text string } }
		}
	}
	if err = json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	for _, c := range r.Candidates {
		for _, p := range c.Content.Parts {
			if p.Text != "" {
				return p.Text, nil
			}
		}
	}
	return "", nil
}
func responsesText(resp *http.Response) (string, error) {
	type response struct {
		Output []struct{ Content []struct{ Text string } }
	}
	var r response
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		s := bufio.NewScanner(resp.Body)
		s.Buffer(make([]byte, 64<<10), 8<<20)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var e struct {
				Type     string
				Response response
			}
			if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &e) == nil && e.Type == "response.completed" {
				r = e.Response
				break
			}
		}
		if err := s.Err(); err != nil {
			return "", err
		}
	} else if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	for _, o := range r.Output {
		for _, c := range o.Content {
			if c.Text != "" {
				return c.Text, nil
			}
		}
	}
	return "", fmt.Errorf("assessment response text missing")
}
func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```json"); i >= 0 {
		s = s[i+7:]
	} else if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

type Telegram struct {
	client              *http.Client
	token, userID, base string
}

func NewTelegram(client *http.Client, token, userID, baseURL string) *Telegram {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	return &Telegram{client, token, userID, strings.TrimRight(baseURL, "/")}
}
func (t *Telegram) Send(ctx context.Context, message string) error {
	if strings.TrimSpace(t.token) == "" || strings.TrimSpace(t.userID) == "" {
		fmt.Fprintln(os.Stderr, "job-alert: Telegram credentials missing, printing to stdout")
		fmt.Println(message)
		return nil
	}
	for _, chunk := range domain.SplitTelegramMessage(message, 4000) {
		body, _ := json.Marshal(map[string]string{"chat_id": t.userID, "text": chunk, "parse_mode": "Markdown", "disable_web_page_preview": "true"})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/bot%s/sendMessage", t.base, t.token), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := t.client.Do(req)
		if err != nil {
			return err
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("telegram status %d: %s", resp.StatusCode, b)
		}
	}
	return nil
}

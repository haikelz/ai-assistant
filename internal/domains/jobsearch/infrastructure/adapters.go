package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"ai-assistant/internal/domains/jobsearch/domain"
)

type AIProviderConfig struct {
	Provider      string
	Model         string
	SumopodAPIKey string
	OpenAIAPIKey  string
	GoogleAPIKey  string
	SumopodURL    string
	OpenAIURL     string
	GoogleURL     string
}

type AIAssessor struct {
	client *http.Client
	config AIProviderConfig
}

func NewAIAssessor(client *http.Client, c AIProviderConfig) *AIAssessor {
	if client == nil {
		client = http.DefaultClient
	}
	return &AIAssessor{
		client: client,
		config: c,
	}
}

const instructions = `Nilai model bisnis setiap perusahaan berdasarkan informasi publik yang benar-benar kamu ketahui. Return HANYA array JSON: {"company":"nama asli","status":"halal|tidak_halal|perlu_riset","reason":"alasan singkat"}. Gunakan tidak_halal untuk bank, asuransi, pinjaman berbunga, riba, atau barang haram; perlu_riset jika ambigu. Jangan berikan fatwa atau markdown.`

type assessment struct {
	Company string
	Status  string
	Reason  string
}

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
	companyKeys := make([]string, 0, len(companies))
	for key := range companies {
		companyKeys = append(companyKeys, key)
	}

	sort.Strings(companyKeys)
	companyList := make([]struct {
		Company string   `json:"company"`
		Roles   []string `json:"roles"`
	}, 0, len(companies))
	for _, key := range companyKeys {
		company := companies[key]
		companyList = append(companyList, struct {
			Company string   `json:"company"`
			Roles   []string `json:"roles"`
		}{
			Company: company.Company,
			Roles:   company.Roles,
		})
	}

	input, err := json.Marshal(companyList)
	if err != nil {
		return out, fmt.Errorf("encode companies for assessment: %w", err)
	}

	text, err := a.request(ctx, instructions, string(input))
	if err != nil {
		return out, err
	}
	var assessments []assessment
	if err = json.Unmarshal([]byte(cleanJSON(text)), &assessments); err != nil {
		return out, err
	}
	byCompany := map[string]assessment{}
	for _, assessment := range assessments {
		if assessment.Status == domain.HalalStatusHalal || assessment.Status == domain.HalalStatusNotHalal || assessment.Status == domain.HalalStatusNeedsReview {
			byCompany[strings.ToLower(strings.TrimSpace(assessment.Company))] = assessment
		}
	}

	for i := range out {
		if assessment, ok := byCompany[strings.ToLower(strings.TrimSpace(out[i].Company))]; ok {
			out[i].HalalStatus = assessment.Status
			out[i].HalalReason = strings.Join(strings.Fields(assessment.Reason), " ")
		}
	}
	return out, nil
}

func (a *AIAssessor) request(ctx context.Context, systemInstructions, input string) (string, error) {
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
		body, err := json.Marshal(map[string]any{
			"system_instruction": map[string]any{
				"parts": []map[string]string{{"text": systemInstructions}},
			},
			"contents": []any{
				map[string]any{
					"role":  "user",
					"parts": []map[string]string{{"text": input}},
				},
			},
		})
		if err != nil {
			return "", fmt.Errorf("encode Google assessment request: %w", err)
		}
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
	body, err := json.Marshal(map[string]string{
		"model":        a.config.Model,
		"instructions": systemInstructions,
		"input":        input,
	})
	if err != nil {
		return "", fmt.Errorf("encode %s assessment request: %w", p, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create %s assessment request: %w", p, err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send %s assessment request: %w", p, err)
	}
	if resp.StatusCode != 200 {
		return "", providerStatusError(p, resp)
	}

	text, responseErr := responsesText(resp)
	closeErr := closeResponseBody(resp)
	return text, errors.Join(responseErr, closeErr)
}

func (a *AIAssessor) doGemini(ctx context.Context, endpoint string, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create Google assessment request: %w", err)
	}
	req.Header.Set("X-Goog-Api-Key", a.config.GoogleAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send Google assessment request: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", providerStatusError("Google", resp)
	}

	payload, err := readAndCloseResponse(resp, 8<<20)
	if err != nil {
		return "", err
	}
	var r struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string
				}
			}
		}
	}
	if err = json.Unmarshal(payload, &r); err != nil {
		return "", fmt.Errorf("decode Google assessment response: %w", err)
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

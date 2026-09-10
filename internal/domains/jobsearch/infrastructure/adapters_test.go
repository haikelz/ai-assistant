package infrastructure

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func TestAIAssessorReadsSumopodResponsesSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"type":"response.completed","response":{"output":[{"content":[{"text":"[{\"company\":\"Company A\",\"status\":\"halal\",\"reason\":\"Teknologi\"}]"}]}]}}`)
	}))
	defer server.Close()
	assessor := NewAIAssessor(server.Client(), AIProviderConfig{Provider: "sumopod", Model: "gpt-5.6-luna", SumopodAPIKey: "key", SumopodURL: server.URL})
	jobs, err := assessor.Assess(t.Context(), []domain.Job{{Company: "Company A", Title: "Engineer"}})
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].HalalStatus != domain.HalalStatusHalal || jobs[0].HalalReason != "Teknologi" {
		t.Fatalf("job=%#v", jobs[0])
	}
}

func TestAIAssessorWithoutModelNeedsResearch(t *testing.T) {
	jobs, err := NewAIAssessor(nil, AIProviderConfig{}).Assess(t.Context(), []domain.Job{{Company: "A"}})
	if err != nil || jobs[0].HalalStatus != domain.HalalStatusNeedsReview {
		t.Fatalf("jobs=%#v err=%v", jobs, err)
	}
}

func TestAIAssessorUsesDynamicOpenAIModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer openai-key" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		var request struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "gpt-dynamic" {
			t.Fatalf("model=%q", request.Model)
		}
		fmt.Fprint(w, `{"output":[{"content":[{"text":"[{\"company\":\"A\",\"status\":\"halal\",\"reason\":\"Teknologi\"}]"}]}]}`)
	}))
	defer server.Close()
	jobs, err := NewAIAssessor(server.Client(), AIProviderConfig{Provider: "openai", Model: "gpt-dynamic", OpenAIAPIKey: "openai-key", OpenAIURL: server.URL}).Assess(t.Context(), []domain.Job{{Company: "A"}})
	if err != nil || jobs[0].HalalStatus != domain.HalalStatusHalal {
		t.Fatalf("jobs=%#v err=%v", jobs, err)
	}
}

func TestAIAssessorUsesGeminiEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-dynamic:generateContent" || r.Header.Get("X-Goog-Api-Key") != "google-key" {
			t.Fatalf("path=%q key=%q", r.URL.Path, r.Header.Get("X-Goog-Api-Key"))
		}
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"[{\"company\":\"A\",\"status\":\"tidak_halal\",\"reason\":\"Bank\"}]"}]}}]}`)
	}))
	defer server.Close()
	jobs, err := NewAIAssessor(server.Client(), AIProviderConfig{Provider: "google", Model: "gemini-dynamic", GoogleAPIKey: "google-key", GoogleURL: server.URL}).Assess(t.Context(), []domain.Job{{Company: "A"}})
	if err != nil || jobs[0].HalalStatus != domain.HalalStatusNotHalal {
		t.Fatalf("jobs=%#v err=%v", jobs, err)
	}
}

func TestAIAssessorRejectsMalformedProviderEndpoint(t *testing.T) {
	assessor := NewAIAssessor(nil, AIProviderConfig{Provider: "sumopod", Model: "gpt-test", SumopodAPIKey: "key", SumopodURL: "://invalid"})
	_, err := assessor.Assess(t.Context(), []domain.Job{{Company: "A"}})
	if err == nil || !strings.Contains(err.Error(), "create sumopod assessment request") {
		t.Fatalf("error=%v", err)
	}
}

func TestAIAssessorOrdersCompaniesDeterministically(t *testing.T) {
	requestInputs := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			requestInputs <- "decode error: " + err.Error()
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		requestInputs <- request.Input
		if _, err := fmt.Fprint(w, `{"output":[{"content":[{"text":"[]"}]}]}`); err != nil {
			return
		}
	}))
	defer server.Close()
	assessor := NewAIAssessor(server.Client(), AIProviderConfig{Provider: "sumopod", Model: "gpt-test", SumopodAPIKey: "key", SumopodURL: server.URL})
	if _, err := assessor.Assess(t.Context(), []domain.Job{{Company: "Z Company"}, {Company: "A Company"}}); err != nil {
		t.Fatal(err)
	}
	if input := <-requestInputs; !strings.HasPrefix(input, `[{"company":"A Company"`) {
		t.Fatalf("input=%s", input)
	}
}

func TestTelegramRejectsMalformedEndpoint(t *testing.T) {
	err := NewTelegram(nil, "token", "user", "://invalid").Send(t.Context(), "message")
	if err == nil || !strings.Contains(err.Error(), "create Telegram request") {
		t.Fatalf("error=%v", err)
	}
}

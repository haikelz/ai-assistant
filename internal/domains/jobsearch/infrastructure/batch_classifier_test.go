package infrastructure

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func TestBatchClassifierLimitsJobsPerAIRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		var payload struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		var input struct {
			Jobs []struct {
				JobID string `json:"job_id"`
			} `json:"jobs"`
		}
		if err := json.Unmarshal([]byte(payload.Input), &input); err != nil {
			t.Fatal(err)
		}
		var results []map[string]any
		for _, job := range input.Jobs {
			results = append(results, map[string]any{"job_id": job.JobID, "relevance_score": 90, "skill_match_score": 80, "halal_status": "halal", "summary": "Cocok"})
		}
		encoded, _ := json.Marshal(map[string]any{"jobs": results})
		text, _ := json.Marshal(string(encoded))
		fmt.Fprintf(response, `{"output":[{"content":[{"text":%s}]}]}`, text)
	}))
	defer server.Close()
	assessor := NewAIAssessor(server.Client(), AIProviderConfig{Provider: "sumopod", Model: "model", SumopodAPIKey: "key", SumopodURL: server.URL})
	classifier := NewBatchClassifier(assessor, 5)
	jobs := make([]domain.NormalizedJob, 12)
	for index := range jobs {
		jobs[index].ID = fmt.Sprintf("job-%d", index)
	}
	classifications, err := classifier.Classify(t.Context(), jobs, domain.Criteria{})
	if err != nil {
		t.Fatal(err)
	}
	if len(classifications) != 12 || calls.Load() != 3 || classifications[0].HalalStatus != domain.HalalStatusHalal {
		t.Fatalf("classifications=%d calls=%d first=%#v", len(classifications), calls.Load(), classifications[0])
	}
}

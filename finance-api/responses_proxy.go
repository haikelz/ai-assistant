package main

import (
	"io"
	"net/http"
)

func (s *server) handleResponsesProxy(w http.ResponseWriter, r *http.Request) {
	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	defer body.Close()

	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.sumopodResponsesURL, body)
	if err != nil {
		http.Error(w, "could not create Sumopod request", http.StatusInternalServerError)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	if authorization := r.Header.Get("Authorization"); authorization != "" {
		request.Header.Set("Authorization", authorization)
	}

	response, err := s.httpClient.Do(request)
	if err != nil {
		http.Error(w, "Sumopod request failed", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	w.WriteHeader(response.StatusCode)
	if _, err := io.Copy(w, response.Body); err != nil {
		return
	}
}

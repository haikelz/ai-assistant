package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
)

const defaultListen = "127.0.0.1:8081"

type lokerRequest struct {
	Query string `json:"query"`
}

func main() {
	addr := os.Getenv("LOKER_ADDR")
	if addr == "" {
		addr = defaultListen
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /loker", handleLoker)
	mux.HandleFunc("GET /health", handleHealth)

	fmt.Fprintf(os.Stderr, "loker-api listening on %s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(fmt.Errorf("loker-api: %w", err))
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func handleLoker(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req lokerRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Query == "" {
		http.Error(w, `{"error":"missing query field"}`, http.StatusBadRequest)
		return
	}

	// Run loker wrapper in background — binary sends result to Telegram
	cmd := exec.Command("/usr/local/bin/loker", req.Query)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "loker-api: failed to start loker: %v\n", err)
		http.Error(w, `{"error":"failed to start search"}`, http.StatusInternalServerError)
		return
	}

	// Don't wait — binary takes a few seconds
	go func() {
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "loker-api: loker exited with error: %v\n", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprint(w, `{"status":"searching"}`)
}

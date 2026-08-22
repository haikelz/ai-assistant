package infrastructure

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-assistant/internal/domains/jobsearch/domain"
)

func TestJSONAlertConfigStorePersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "job-alert.json")
	written := domain.AlertConfig{Query: "Software Engineer | go | 1-3 | Jakarta", UpdatedAt: time.Now().UTC()}
	if err := NewJSONAlertConfigStore(path).Save(t.Context(), written); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewJSONAlertConfigStore(path).Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Query != written.Query || !loaded.UpdatedAt.Equal(written.UpdatedAt) {
		t.Fatalf("loaded=%#v", loaded)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
}

func TestJSONAlertConfigStoreReportsMissingAndCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "job-alert.json")
	store := NewJSONAlertConfigStore(path)
	if _, err := store.Load(t.Context()); !errors.Is(err, domain.ErrAlertConfigNotFound) {
		t.Fatalf("missing err=%v", err)
	}
	if err := os.WriteFile(path, []byte(`{"query":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(t.Context()); err == nil {
		t.Fatal("corrupt configuration should fail")
	}
}

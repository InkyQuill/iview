package source

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestResolveDownloadsToTemporaryFileAndCleanupRemovesIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "not image yet")
	}))
	defer server.Close()

	resolved, err := Resolve(context.Background(), server.URL, 1024)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if !resolved.Temporary {
		t.Fatal("expected downloaded source to be temporary")
	}
	if _, err := os.Stat(resolved.Path); err != nil {
		t.Fatalf("expected temp file to exist: %v", err)
	}

	resolved.Cleanup()
	if _, err := os.Stat(resolved.Path); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be removed, stat error: %v", err)
	}
}

func TestResolveRejectsDownloadOverLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "1234567890")
	}))
	defer server.Close()

	_, err := Resolve(context.Background(), server.URL, 4)
	if err == nil {
		t.Fatal("expected error")
	}
}

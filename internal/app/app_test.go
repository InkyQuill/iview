package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/InkyQuill/iview/internal/render"
	"github.com/InkyQuill/iview/internal/source"
	"github.com/InkyQuill/iview/internal/terminal"
)

func TestRunFallsBackToCellsForUnsupportedTerminal(t *testing.T) {
	path := writeAppPNG(t)

	var stdout bytes.Buffer
	cfg := validAppConfig(path)
	cfg.Env = []string{"TERM=xterm-256color"}
	cfg.Stdout = &stdout
	cfg.QuerySize = func(string) (terminal.Size, error) {
		return terminal.Size{
			Rows:         10,
			Columns:      20,
			WidthPixels:  200,
			HeightPixels: 100,
		}, nil
	}

	err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if stdout.Len() == 0 {
		t.Fatal("expected fallback cell output")
	}
}

func TestRunRejectsInvalidNormalizationConfigBeforeWork(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			name: "max pixels",
			edit: func(cfg *Config) {
				cfg.MaxPixels = 0
			},
			want: "max-pixels must be greater than zero",
		},
		{
			name: "max output bytes",
			edit: func(cfg *Config) {
				cfg.MaxOutputBytes = 0
			},
			want: "max-output-bytes must be greater than zero",
		},
		{
			name: "conversion timeout",
			edit: func(cfg *Config) {
				cfg.ConversionTimeout = 0
			},
			want: "conversion-timeout must be greater than zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validAppConfig("image.png")
			cfg.Env = []string{"TERM=xterm-256color"}
			cfg.RendererMode = render.ModeKitty
			cfg.Resolve = func(context.Context, string, int64) (source.Resolved, error) {
				t.Fatal("Resolve should not be called for invalid config")
				return source.Resolved{}, nil
			}
			cfg.QuerySize = func(string) (terminal.Size, error) {
				t.Fatal("QuerySize should not be called for invalid config")
				return terminal.Size{}, nil
			}
			tt.edit(&cfg)

			err := Run(context.Background(), cfg)
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Error() != tt.want {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestRunRejectsInvalidMaxBytesBeforeWork(t *testing.T) {
	cfg := validAppConfig("image.png")
	cfg.MaxBytes = 0
	cfg.Resolve = func(context.Context, string, int64) (source.Resolved, error) {
		t.Fatal("Resolve should not be called for invalid config")
		return source.Resolved{}, nil
	}
	cfg.QuerySize = func(string) (terminal.Size, error) {
		t.Fatal("QuerySize should not be called for invalid config")
		return terminal.Size{}, nil
	}

	err := Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "max-bytes must be greater than zero" {
		t.Fatalf("error = %q, want max-bytes validation", err.Error())
	}
}

func TestRunForceAutoWarnsAndUsesKitty(t *testing.T) {
	path := writeAppPNG(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cfg := validAppConfig(path)
	cfg.Env = []string{"TERM=xterm-256color"}
	cfg.Stdout = &stdout
	cfg.Stderr = &stderr
	cfg.Force = true
	cfg.QuerySize = func(string) (terminal.Size, error) {
		return terminal.Size{
			Rows:         10,
			Columns:      20,
			WidthPixels:  200,
			HeightPixels: 100,
		}, nil
	}

	err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "\x1b_G") {
		t.Fatalf("expected Kitty graphics escape, got %q", stdout.String())
	}
	const wantWarning = "iview: warning: forcing Kitty graphics output for an unrecognized terminal\n"
	if stderr.String() != wantWarning {
		t.Fatalf("stderr = %q, want %q", stderr.String(), wantWarning)
	}
}

func TestRunRejectsDownloadedNonImageBeforeRendering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "plain text")
	}))
	defer server.Close()

	var stdout bytes.Buffer
	cfg := validAppConfig(server.URL)
	cfg.Env = []string{"TERM_PROGRAM=ghostty"}
	cfg.Stdout = &stdout
	cfg.QuerySize = func(string) (terminal.Size, error) {
		t.Fatal("QuerySize should not be called for invalid images")
		return terminal.Size{}, nil
	}

	err := Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not an image") {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no graphics output, got %q", stdout.String())
	}
}

func TestRunRemovesTemporaryFileWhenValidationFails(t *testing.T) {
	tmp, err := os.CreateTemp("", "iview-*.download")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	path := tmp.Name()
	if _, err := tmp.WriteString("plain text"); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(path)
	})

	var cleanupCalls int
	cfg := validAppConfig("https://example.test/not-image")
	cfg.Env = []string{"TERM_PROGRAM=ghostty"}
	cfg.Resolve = func(context.Context, string, int64) (source.Resolved, error) {
		return source.Resolved{
			Path:      path,
			Temporary: true,
			Cleanup: func() {
				cleanupCalls++
				_ = os.Remove(path)
			},
		}, nil
	}
	cfg.QuerySize = func(string) (terminal.Size, error) {
		t.Fatal("QuerySize should not be called for invalid images")
		return terminal.Size{}, nil
	}

	err = Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error")
	}

	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanupCalls)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected temporary file to be removed, stat error: %v", err)
	}
}

func validAppConfig(args ...string) Config {
	return Config{
		Args:              args,
		MaxBytes:          1024,
		MaxPixels:         100_000_000,
		MaxOutputBytes:    100 << 20,
		ConversionTimeout: 5 * time.Second,
	}
}

func writeAppPNG(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "image.png")
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create png: %v", err)
	}
	defer func() { _ = f.Close() }()

	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return path
}

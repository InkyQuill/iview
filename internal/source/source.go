package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

type Resolved struct {
	Path      string
	Temporary bool
	Cleanup   func()
}

func Resolve(ctx context.Context, input string, maxBytes int64) (Resolved, error) {
	u, err := url.Parse(input)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return download(ctx, input, maxBytes)
	}
	if err == nil && u.Scheme != "" && u.Scheme != "file" {
		return Resolved{}, fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}

	path := input
	if u.Scheme == "file" {
		path = u.Path
	}
	if path == "" {
		return Resolved{}, errors.New("empty image path")
	}
	if info, err := os.Stat(path); err != nil {
		return Resolved{}, fmt.Errorf("stat image: %w", err)
	} else if info.IsDir() {
		return Resolved{}, fmt.Errorf("image path is a directory: %s", path)
	}
	return Resolved{
		Path:      path,
		Temporary: false,
		Cleanup:   func() {},
	}, nil
}

func download(ctx context.Context, rawURL string, maxBytes int64) (Resolved, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("redirected to unsupported scheme %q", req.URL.Scheme)
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Resolved{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "iview/0.1")

	resp, err := client.Do(req)
	if err != nil {
		return Resolved{}, fmt.Errorf("download image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Resolved{}, fmt.Errorf("download image: unexpected HTTP status %s", resp.Status)
	}
	if resp.ContentLength > maxBytes {
		return Resolved{}, fmt.Errorf("download image: content length %d exceeds limit %d", resp.ContentLength, maxBytes)
	}

	tmp, err := os.CreateTemp("", "iview-*.download")
	if err != nil {
		return Resolved{}, fmt.Errorf("create temp file: %w", err)
	}
	path := tmp.Name()
	cleanup := func() {
		_ = os.Remove(path)
	}

	written, err := io.Copy(tmp, io.LimitReader(resp.Body, maxBytes+1))
	closeErr := tmp.Close()
	if err != nil {
		cleanup()
		return Resolved{}, fmt.Errorf("write temp image: %w", err)
	}
	if closeErr != nil {
		cleanup()
		return Resolved{}, fmt.Errorf("close temp image: %w", closeErr)
	}
	if written > maxBytes {
		cleanup()
		return Resolved{}, fmt.Errorf("download image: size exceeds limit %d", maxBytes)
	}

	return Resolved{
		Path:      path,
		Temporary: true,
		Cleanup:   cleanup,
	}, nil
}

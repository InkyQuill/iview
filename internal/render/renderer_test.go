package render

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    Mode
		wantErr bool
	}{
		{name: "empty is auto", value: "", want: ModeAuto},
		{name: "auto", value: "auto", want: ModeAuto},
		{name: "case insensitive", value: "KiTtY", want: ModeKitty},
		{name: "trimmed", value: " cells ", want: ModeCells},
		{name: "unknown", value: "bad", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMode(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ParseMode returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMode returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseMode = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelectAutoChoosesKittyBeforeCells(t *testing.T) {
	renderer, err := Select(ModeAuto, []string{"KITTY_WINDOW_ID=1"}, false, missingLookup)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if renderer.Name() != "kitty" {
		t.Fatalf("renderer = %q, want kitty", renderer.Name())
	}
}

func TestSelectAutoChoosesITerm2(t *testing.T) {
	renderer, err := Select(ModeAuto, []string{"TERM_PROGRAM=iTerm.app"}, false, missingLookup)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if renderer.Name() != "iterm2" {
		t.Fatalf("renderer = %q, want iterm2", renderer.Name())
	}
}

func TestSelectAutoFallsBackToCells(t *testing.T) {
	renderer, err := Select(ModeAuto, []string{"TERM=xterm-256color"}, false, missingLookup)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if renderer.Name() != "cells" {
		t.Fatalf("renderer = %q, want cells", renderer.Name())
	}
}

func TestSelectForceAutoReturnsKitty(t *testing.T) {
	renderer, err := Select(ModeAuto, []string{"TERM=xterm-256color"}, true, missingLookup)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if renderer.Name() != "kitty" {
		t.Fatalf("renderer = %q, want kitty", renderer.Name())
	}
}

func TestSelectExplicitUnsupportedMentionsForce(t *testing.T) {
	_, err := Select(ModeKitty, []string{"TERM=xterm-256color"}, false, missingLookup)
	if err == nil {
		t.Fatal("Select returned nil error")
	}
	if err.Error() != `renderer "kitty" is not supported by this terminal; use --force` {
		t.Fatalf("error = %q", err.Error())
	}
}

func missingLookup(string) (string, error) {
	return "", errors.New("missing")
}

func testPNG(t *testing.T, c color.Color) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, c)

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return out.Bytes()
}

func testPatternPNG(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8((x*17 + y*31) % 256),
				G: uint8((x*47 + y*13) % 256),
				B: uint8((x*19 + y*53) % 256),
				A: 255,
			})
		}
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return out.Bytes()
}

func truncatedPNGPassingDecodeConfig(t *testing.T) []byte {
	t.Helper()

	pngBytes := testPNG(t, color.RGBA{A: 255})
	if len(pngBytes) < 33 {
		t.Fatalf("encoded png length = %d, want at least IHDR", len(pngBytes))
	}
	truncated := append([]byte(nil), pngBytes[:33]...)
	if _, err := png.DecodeConfig(bytes.NewReader(truncated)); err != nil {
		t.Fatalf("truncated png should pass DecodeConfig: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(truncated)); err == nil {
		t.Fatal("truncated png should fail full Decode")
	}
	return truncated
}

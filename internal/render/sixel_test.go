package render

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"iview/internal/imageio"
	"iview/internal/terminal"
)

func TestSixelRendererSupportedWithFakeLookup(t *testing.T) {
	renderer := SixelRenderer{
		Lookup: func(name string) (string, error) {
			if name != "img2sixel" {
				t.Fatalf("lookup name = %q, want img2sixel", name)
			}
			return "/usr/bin/img2sixel", nil
		},
	}

	if !renderer.Supported([]string{"DEC_IMAGE_PROTOCOL=sixel"}) {
		t.Fatal("Supported returned false")
	}
}

func TestSixelRendererUnsupportedWithoutTool(t *testing.T) {
	renderer := SixelRenderer{Lookup: missingLookup}

	if renderer.Supported([]string{"TERM=xterm-sixel"}) {
		t.Fatal("Supported returned true")
	}
}

func TestSixelRendererUnsupportedWithoutSixelEnvironment(t *testing.T) {
	renderer := SixelRenderer{
		Lookup: func(string) (string, error) {
			return "/usr/bin/img2sixel", nil
		},
	}

	if renderer.Supported([]string{"TERM=xterm-256color"}) {
		t.Fatal("Supported returned true")
	}
}

func TestSixelRendererRenderMissingTool(t *testing.T) {
	pngBytes := testPNG(t, color.RGBA{B: 255, A: 255})
	renderer := SixelRenderer{
		Lookup: func(string) (string, error) {
			return "", errors.New("not found")
		},
	}

	err := renderer.Render(
		io.Discard,
		imageio.NormalizedImage{PNG: pngBytes},
		terminal.Placement{Columns: 1, Rows: 1},
	)
	if err == nil {
		t.Fatal("Render returned nil error")
	}
	want := "sixel rendering requires img2sixel. Install libsixel or use --renderer=cells"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestSixelRendererInvalidPNGBeforeLookup(t *testing.T) {
	var out bytes.Buffer
	renderer := SixelRenderer{
		Lookup: func(string) (string, error) {
			t.Fatal("lookup should not be called for invalid png")
			return "", nil
		},
	}

	err := renderer.Render(
		&out,
		imageio.NormalizedImage{PNG: []byte("not a png")},
		terminal.Placement{Columns: 1, Rows: 1},
	)
	if err == nil {
		t.Fatal("Render returned nil error")
	}
	if !strings.Contains(err.Error(), "invalid png payload") {
		t.Fatalf("error = %q, want invalid png payload", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("output length = %d, want 0", out.Len())
	}
}

func TestSixelRendererRenderUsesPixelResizeArgsAndCleansTempPNG(t *testing.T) {
	pngBytes := testPNG(t, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var out bytes.Buffer
	var tempPath string

	renderer := SixelRenderer{
		Lookup: func(name string) (string, error) {
			if name != "img2sixel" {
				t.Fatalf("lookup name = %q, want img2sixel", name)
			}
			return "/usr/bin/img2sixel", nil
		},
		runner: func(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error {
			if ctx == nil {
				t.Fatal("ctx is nil")
			}
			if exe != "/usr/bin/img2sixel" {
				t.Fatalf("exe = %q, want /usr/bin/img2sixel", exe)
			}
			wantPrefix := []string{"-w", "120", "-h", "80"}
			if len(args) != 5 || !reflect.DeepEqual(args[:4], wantPrefix) {
				t.Fatalf("args = %#v, want prefix %#v and temp path", args, wantPrefix)
			}

			tempPath = args[4]
			got, err := os.ReadFile(tempPath)
			if err != nil {
				t.Fatalf("read temp png: %v", err)
			}
			if !bytes.Equal(got, pngBytes) {
				t.Fatal("temp png content did not match normalized png")
			}
			if _, err := stdout.Write([]byte("sixel-output")); err != nil {
				t.Fatalf("write stdout: %v", err)
			}
			return nil
		},
	}

	err := renderer.Render(
		&out,
		imageio.NormalizedImage{PNG: pngBytes},
		terminal.Placement{Columns: 2, Rows: 3, WidthPixels: 120, HeightPixels: 80},
	)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if out.String() != "sixel-output" {
		t.Fatalf("output = %q, want sixel-output", out.String())
	}
	if tempPath == "" {
		t.Fatal("runner did not record temp path")
	}
	if _, err := os.Stat(tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temp path still exists or unexpected stat error: %v", err)
	}
}

func TestSixelRendererRenderOmitsResizeArgsWithoutPixelDimensions(t *testing.T) {
	pngBytes := testPNG(t, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var tempPath string
	placement, err := terminal.FitToViewport(4000, 2000, terminal.Size{
		Rows:    50,
		Columns: 100,
	}, false)
	if err != nil {
		t.Fatalf("FitToViewport returned error: %v", err)
	}
	if placement.WidthPixels != 0 || placement.HeightPixels != 0 {
		t.Fatalf("placement pixel dimensions = %dx%d, want zero", placement.WidthPixels, placement.HeightPixels)
	}

	renderer := SixelRenderer{
		Lookup: func(string) (string, error) {
			return "/usr/bin/img2sixel", nil
		},
		runner: func(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error {
			if len(args) != 1 {
				t.Fatalf("args = %#v, want only temp path", args)
			}
			tempPath = args[0]
			if args[0] == "2" || args[0] == "3" {
				t.Fatalf("cell dimensions were passed as resize args: %#v", args)
			}
			return nil
		},
	}

	err = renderer.Render(
		io.Discard,
		imageio.NormalizedImage{PNG: pngBytes},
		placement,
	)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if tempPath == "" {
		t.Fatal("runner did not record temp path")
	}
}

func TestSixelRendererConverterFailureBoundsStderr(t *testing.T) {
	pngBytes := testPNG(t, color.RGBA{A: 255})
	renderer := SixelRenderer{
		Lookup: func(string) (string, error) {
			return "/usr/bin/img2sixel", nil
		},
		runner: func(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error {
			_, _ = stderr.Write([]byte(strings.Repeat("x", maxSixelStderrBytes+128)))
			return errors.New("boom")
		},
	}

	err := renderer.Render(
		io.Discard,
		imageio.NormalizedImage{PNG: pngBytes},
		terminal.Placement{Columns: 1, Rows: 1},
	)
	if err == nil {
		t.Fatal("Render returned nil error")
	}
	message := err.Error()
	if !strings.Contains(message, stderrTruncatedMarker) {
		t.Fatalf("error = %q, want truncation marker", message)
	}
	if !strings.Contains(message, strings.Repeat("x", maxSixelStderrBytes)+stderrTruncatedMarker) {
		t.Fatalf("error does not contain bounded stderr followed by marker")
	}
	if strings.Contains(message, strings.Repeat("x", maxSixelStderrBytes+1)) {
		t.Fatalf("error contains more than bounded stderr")
	}
}

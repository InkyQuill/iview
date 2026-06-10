package render

import (
	"bytes"
	"encoding/base64"
	"image/color"
	"strings"
	"testing"

	"github.com/InkyQuill/iview/internal/imageio"
	"github.com/InkyQuill/iview/internal/terminal"
)

func TestKittyPNGWritesSingleChunkPlacement(t *testing.T) {
	var out bytes.Buffer
	pngBytes := testPNG(t, color.RGBA{R: 255, A: 255})

	err := KittyPNG(&out, pngBytes, 2, 3)
	if err != nil {
		t.Fatalf("KittyPNG returned error: %v", err)
	}

	want := "\x1b_Ga=T,f=100,c=2,r=3,m=0;" + base64.StdEncoding.EncodeToString(pngBytes) + "\x1b\\\n"
	if out.String() != want {
		t.Fatalf("unexpected output\nwant: %q\n got: %q", want, out.String())
	}
}

func TestKittyPNGChunksLargePayload(t *testing.T) {
	var out bytes.Buffer
	payload := testPatternPNG(t, 128, 128)
	if len(payload) <= maxEncodedChunk/4*3 {
		t.Fatalf("test png length = %d, want larger than one raw chunk", len(payload))
	}

	err := KittyPNG(&out, payload, 10, 4)
	if err != nil {
		t.Fatalf("KittyPNG returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "\x1b_Ga=T,f=100,c=10,r=4,m=1;") {
		t.Fatalf("first chunk did not advertise continuation: %q", got[:80])
	}
	if !strings.Contains(got, "\x1b_Gm=0;") {
		t.Fatalf("last chunk did not terminate continuation")
	}
}

func TestKittyPNGInvalidPayloadWritesNoOutput(t *testing.T) {
	var out bytes.Buffer

	err := KittyPNG(&out, []byte("not a png"), 2, 3)
	if err == nil {
		t.Fatal("KittyPNG returned nil error")
	}
	if !strings.Contains(err.Error(), "invalid png payload") {
		t.Fatalf("error = %q, want invalid png payload", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("output length = %d, want 0", out.Len())
	}
}

func TestKittyRendererRenderDelegatesToKittyPNG(t *testing.T) {
	var out bytes.Buffer
	pngBytes := testPNG(t, color.RGBA{R: 255, A: 255})

	err := KittyRenderer{}.Render(
		&out,
		imageio.NormalizedImage{PNG: pngBytes},
		terminal.Placement{Columns: 2, Rows: 3},
	)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	want := "\x1b_Ga=T,f=100,c=2,r=3,m=0;" + base64.StdEncoding.EncodeToString(pngBytes) + "\x1b\\\n"
	if out.String() != want {
		t.Fatalf("unexpected output\nwant: %q\n got: %q", want, out.String())
	}
}

func TestKittyRendererInvalidPNGWritesNoOutput(t *testing.T) {
	var out bytes.Buffer

	err := KittyRenderer{}.Render(
		&out,
		imageio.NormalizedImage{PNG: []byte("not a png")},
		terminal.Placement{Columns: 2, Rows: 3},
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

func TestKittyRendererTruncatedPNGWritesNoOutput(t *testing.T) {
	var out bytes.Buffer

	err := KittyRenderer{}.Render(
		&out,
		imageio.NormalizedImage{PNG: truncatedPNGPassingDecodeConfig(t)},
		terminal.Placement{Columns: 2, Rows: 3},
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

func TestKittyRendererSupported(t *testing.T) {
	tests := []struct {
		name string
		env  []string
	}{
		{name: "kitty", env: []string{"KITTY_WINDOW_ID=1"}},
		{name: "ghostty", env: []string{"TERM_PROGRAM=ghostty"}},
		{name: "wezterm", env: []string{"TERM_PROGRAM=WezTerm"}},
		{name: "konsole", env: []string{"KONSOLE_VERSION=240800"}},
		{name: "term contains kitty", env: []string{"TERM=xterm-kitty"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !(KittyRenderer{}).Supported(tt.env) {
				t.Fatalf("Supported(%v) = false, want true", tt.env)
			}
		})
	}
}

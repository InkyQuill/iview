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

func TestITerm2RendererSupported(t *testing.T) {
	if !(ITerm2Renderer{}).Supported([]string{"TERM_PROGRAM=iterm.APP"}) {
		t.Fatal("Supported returned false")
	}
}

func TestITerm2RendererRender(t *testing.T) {
	var out bytes.Buffer
	payload := testPNG(t, color.RGBA{G: 255, A: 255})

	err := ITerm2Renderer{}.Render(
		&out,
		imageio.NormalizedImage{PNG: payload},
		terminal.Placement{Columns: 2, Rows: 3},
	)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	got := out.String()
	wantPrefix := "\x1b]1337;File=inline=1;width=2;height=3;preserveAspectRatio=1:"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("output prefix = %q, want prefix %q", got, wantPrefix)
	}
	if !strings.Contains(got, base64.StdEncoding.EncodeToString(payload)) {
		t.Fatalf("output does not contain base64 payload: %q", got)
	}
}

func TestITerm2RendererInvalidPNGWritesNoOutput(t *testing.T) {
	var out bytes.Buffer

	err := ITerm2Renderer{}.Render(
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

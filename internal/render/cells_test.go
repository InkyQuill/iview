package render

import (
	"bytes"
	"image/color"
	"testing"

	"iview/internal/imageio"
	"iview/internal/terminal"
)

func TestCellsRendererRenderOnePixelPNG(t *testing.T) {
	pngBytes := testPNG(t, color.RGBA{R: 255, A: 255})

	var out bytes.Buffer
	err := CellsRenderer{}.Render(
		&out,
		imageio.NormalizedImage{PNG: pngBytes},
		terminal.Placement{Columns: 1, Rows: 1},
	)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	want := "\x1b[38;2;255;0;0m█\x1b[0m\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestCellsRendererRejectsInvalidPlacement(t *testing.T) {
	err := CellsRenderer{}.Render(
		bytes.NewBuffer(nil),
		imageio.NormalizedImage{PNG: []byte("png")},
		terminal.Placement{Columns: 0, Rows: 1},
	)
	if err == nil {
		t.Fatal("Render returned nil error")
	}
}

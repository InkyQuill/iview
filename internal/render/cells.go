package render

import (
	"bytes"
	"fmt"
	"image/png"
	"io"

	"github.com/InkyQuill/iview/internal/imageio"
	"github.com/InkyQuill/iview/internal/terminal"
)

type CellsRenderer struct{}

func (CellsRenderer) Name() string {
	return "cells"
}

func (CellsRenderer) Supported([]string) bool {
	return true
}

func (CellsRenderer) Render(w io.Writer, img imageio.NormalizedImage, placement terminal.Placement) error {
	if err := validateRenderInput(img, placement); err != nil {
		return err
	}

	decoded, err := png.Decode(bytes.NewReader(img.PNG))
	if err != nil {
		return fmt.Errorf("decode png for cell rendering: %w", err)
	}

	bounds := decoded.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return fmt.Errorf("decoded png dimensions must be positive")
	}

	for row := range placement.Rows {
		y := bounds.Min.Y + row*height/placement.Rows
		for column := range placement.Columns {
			x := bounds.Min.X + column*width/placement.Columns
			r, g, b, _ := decoded.At(x, y).RGBA()
			if _, err := fmt.Fprintf(w, "\x1b[38;2;%d;%d;%dm█", r>>8, g>>8, b>>8); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, "\x1b[0m\n"); err != nil {
			return err
		}
	}
	return nil
}

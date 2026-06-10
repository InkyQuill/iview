package render

import (
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"iview/internal/imageio"
	"iview/internal/terminal"
)

const maxEncodedChunk = 4096

type KittyRenderer struct{}

func (KittyRenderer) Name() string {
	return "kitty"
}

func (KittyRenderer) Supported(env []string) bool {
	values := envMap(env)
	term := strings.ToLower(values["TERM"])
	termProgram := strings.ToLower(values["TERM_PROGRAM"])

	hasGhostty := values["GHOSTTY_RESOURCES_DIR"] != "" || termProgram == "ghostty"
	hasWezTerm := values["WEZTERM_EXECUTABLE"] != "" || strings.Contains(termProgram, "wezterm")
	return values["KITTY_WINDOW_ID"] != "" ||
		hasGhostty ||
		hasWezTerm ||
		values["KONSOLE_VERSION"] != "" ||
		strings.Contains(term, "kitty")
}

func (KittyRenderer) Render(w io.Writer, img imageio.NormalizedImage, placement terminal.Placement) error {
	if err := validateRenderInput(img, placement); err != nil {
		return err
	}
	return KittyPNG(w, img.PNG, placement.Columns, placement.Rows)
}

func KittyPNG(w io.Writer, pngBytes []byte, columns, rows int) error {
	if err := validateRenderInput(
		imageio.NormalizedImage{PNG: pngBytes},
		terminal.Placement{Columns: columns, Rows: rows},
	); err != nil {
		return err
	}
	return writeKittyPNGChunks(w, pngBytes, columns, rows)
}

func writeKittyPNGChunks(w io.Writer, pngBytes []byte, columns, rows int) error {
	encoder := base64.StdEncoding
	rawChunk := maxEncodedChunk / 4 * 3
	for offset := 0; offset < len(pngBytes); offset += rawChunk {
		end := min(offset+rawChunk, len(pngBytes))
		chunk := pngBytes[offset:end]
		encoded := make([]byte, encoder.EncodedLen(len(chunk)))
		encoder.Encode(encoded, chunk)

		more := 0
		if end < len(pngBytes) {
			more = 1
		}

		control := fmt.Sprintf("m=%d", more)
		if offset == 0 {
			control = fmt.Sprintf("a=T,f=100,c=%d,r=%d,m=%d", columns, rows, more)
		}
		if _, err := fmt.Fprintf(w, "\x1b_G%s;%s\x1b\\", control, encoded); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprint(w, "\n"); err != nil {
		return err
	}
	return nil
}

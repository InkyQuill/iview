package render

import (
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"iview/internal/imageio"
	"iview/internal/terminal"
)

type ITerm2Renderer struct{}

func (ITerm2Renderer) Name() string {
	return "iterm2"
}

func (ITerm2Renderer) Supported(env []string) bool {
	values := envMap(env)
	return strings.EqualFold(values["TERM_PROGRAM"], "iTerm.app")
}

func (ITerm2Renderer) Render(w io.Writer, img imageio.NormalizedImage, placement terminal.Placement) error {
	if err := validateRenderInput(img, placement); err != nil {
		return err
	}

	payload := base64.StdEncoding.EncodeToString(img.PNG)
	_, err := fmt.Fprintf(
		w,
		"\x1b]1337;File=inline=1;width=%d;height=%d;preserveAspectRatio=1:%s\a\n",
		placement.Columns,
		placement.Rows,
		payload,
	)
	return err
}

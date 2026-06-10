package render

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"iview/internal/imageio"
	"iview/internal/terminal"
)

const (
	sixelRenderTimeout    = 5 * time.Second
	maxSixelStderrBytes   = 4096
	stderrTruncatedMarker = "... [stderr truncated]"
)

type sixelCommandRunner func(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error

type SixelRenderer struct {
	Lookup ToolLookup
	runner sixelCommandRunner
}

func (r SixelRenderer) Name() string {
	return "sixel"
}

func (r SixelRenderer) Supported(env []string) bool {
	if !advertisesSixel(env) {
		return false
	}
	exe, err := r.lookup()("img2sixel")
	return err == nil && exe != ""
}

func (r SixelRenderer) Render(w io.Writer, img imageio.NormalizedImage, placement terminal.Placement) error {
	if err := validateRenderInput(img, placement); err != nil {
		return err
	}

	exe, err := r.img2sixelPath()
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "iview-sixel-*.png")
	if err != nil {
		return fmt.Errorf("create sixel temp png: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(img.PNG); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write sixel temp png: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close sixel temp png: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), sixelRenderTimeout)
	defer cancel()

	stderr := &boundedStderr{limit: maxSixelStderrBytes}
	args := sixelArgs(tmpPath, placement)
	if err := r.commandRunner()(ctx, exe, args, w, stderr); err != nil {
		return sixelRenderError(ctx, err, stderr.String())
	}
	return nil
}

func sixelArgs(path string, placement terminal.Placement) []string {
	if placement.WidthPixels <= 0 || placement.HeightPixels <= 0 {
		return []string{path}
	}
	return []string{
		"-w",
		strconv.Itoa(placement.WidthPixels),
		"-h",
		strconv.Itoa(placement.HeightPixels),
		path,
	}
}

func (r SixelRenderer) img2sixelPath() (string, error) {
	exe, err := r.lookup()("img2sixel")
	if err != nil || exe == "" {
		return "", fmt.Errorf("sixel rendering requires img2sixel. Install libsixel or use --renderer=cells")
	}
	if filepath.IsAbs(exe) {
		return exe, nil
	}
	abs, err := filepath.Abs(exe)
	if err != nil {
		return "", fmt.Errorf("resolve img2sixel path: %w", err)
	}
	return abs, nil
}

func (r SixelRenderer) lookup() ToolLookup {
	if r.Lookup != nil {
		return r.Lookup
	}
	return exec.LookPath
}

func (r SixelRenderer) commandRunner() sixelCommandRunner {
	if r.runner != nil {
		return r.runner
	}
	return runSixelCommand
}

func runSixelCommand(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func sixelRenderError(ctx context.Context, err error, stderr string) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("img2sixel timed out after %s", sixelRenderTimeout)
	}
	message := strings.TrimSpace(stderr)
	if message == "" {
		return fmt.Errorf("img2sixel failed: %w", err)
	}
	return fmt.Errorf("img2sixel failed: %w: %s", err, message)
}

type boundedStderr struct {
	strings.Builder
	limit     int
	truncated bool
}

func (b *boundedStderr) Write(p []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining > 0 {
		if len(p) <= remaining {
			_, _ = b.Builder.Write(p)
		} else {
			_, _ = b.Builder.Write(p[:remaining])
			b.truncated = true
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return len(p), nil
}

func (b *boundedStderr) String() string {
	s := b.Builder.String()
	if b.truncated {
		return s + stderrTruncatedMarker
	}
	return s
}

func advertisesSixel(env []string) bool {
	values := envMap(env)
	return strings.EqualFold(values["TERM"], "xterm-sixel") ||
		strings.EqualFold(values["DEC_IMAGE_PROTOCOL"], "sixel") ||
		values["SIXEL"] == "1"
}

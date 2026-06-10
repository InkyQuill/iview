package render

import (
	"bytes"
	"fmt"
	"image/png"
	"io"
	"os/exec"
	"strings"

	"github.com/InkyQuill/iview/internal/imageio"
	"github.com/InkyQuill/iview/internal/terminal"
)

type Mode string

const (
	ModeAuto   Mode = "auto"
	ModeKitty  Mode = "kitty"
	ModeITerm2 Mode = "iterm2"
	ModeSixel  Mode = "sixel"
	ModeCells  Mode = "cells"
)

type Renderer interface {
	Name() string
	Supported(env []string) bool
	Render(w io.Writer, img imageio.NormalizedImage, placement terminal.Placement) error
}

type ToolLookup func(string) (string, error)

func ParseMode(value string) (Mode, error) {
	mode := Mode(strings.ToLower(strings.TrimSpace(value)))
	if mode == "" {
		return ModeAuto, nil
	}

	switch mode {
	case ModeAuto, ModeKitty, ModeITerm2, ModeSixel, ModeCells:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported renderer %q", value)
	}
}

func Select(mode Mode, env []string, force bool, lookup ToolLookup) (Renderer, error) {
	if lookup == nil {
		lookup = exec.LookPath
	}

	renderers := []Renderer{
		KittyRenderer{},
		ITerm2Renderer{},
		SixelRenderer{Lookup: lookup},
		CellsRenderer{},
	}

	if mode == ModeAuto && force {
		return KittyRenderer{}, nil
	}

	if mode != ModeAuto {
		for _, renderer := range renderers {
			if renderer.Name() != string(mode) {
				continue
			}
			if force || renderer.Supported(env) {
				return renderer, nil
			}
			return nil, fmt.Errorf("renderer %q is not supported by this terminal; use --force", mode)
		}
		return nil, fmt.Errorf("unsupported renderer %q", mode)
	}

	for _, renderer := range renderers {
		if renderer.Supported(env) {
			return renderer, nil
		}
	}
	return nil, fmt.Errorf("no supported renderer found")
}

func envMap(env []string) map[string]string {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		values[key] = value
	}
	return values
}

func validatePNGPlacement(png []byte, placement terminal.Placement) error {
	if len(png) == 0 {
		return fmt.Errorf("empty png payload")
	}
	if placement.Columns <= 0 || placement.Rows <= 0 {
		return fmt.Errorf("placement dimensions must be positive")
	}
	return nil
}

func validateRenderInput(img imageio.NormalizedImage, placement terminal.Placement) error {
	if err := validatePNGPlacement(img.PNG, placement); err != nil {
		return err
	}
	if _, err := png.Decode(bytes.NewReader(img.PNG)); err != nil {
		return fmt.Errorf("invalid png payload: %w", err)
	}
	return nil
}

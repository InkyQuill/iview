package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/InkyQuill/iview/internal/imageio"
	"github.com/InkyQuill/iview/internal/render"
	"github.com/InkyQuill/iview/internal/source"
	"github.com/InkyQuill/iview/internal/terminal"
)

type Config struct {
	Args              []string
	Env               []string
	Stdout            io.Writer
	Stderr            io.Writer
	TTYPath           string
	MaxBytes          int64
	MaxPixels         int
	MaxOutputBytes    int64
	ConversionTimeout time.Duration
	RendererMode      render.Mode
	ScaleUp           bool
	Force             bool
	ShowUsage         func()
	QuerySize         func(string) (terminal.Size, error)
	Resolve           func(context.Context, string, int64) (source.Resolved, error)
}

func Run(ctx context.Context, cfg Config) error {
	if len(cfg.Args) != 1 {
		if cfg.ShowUsage != nil {
			cfg.ShowUsage()
		}
		return errors.New("expected exactly one image path or URL")
	}
	if cfg.Stdout == nil {
		cfg.Stdout = io.Discard
	}
	if cfg.MaxBytes <= 0 {
		return errors.New("max-bytes must be greater than zero")
	}
	if cfg.MaxPixels <= 0 {
		return errors.New("max-pixels must be greater than zero")
	}
	if cfg.MaxOutputBytes <= 0 {
		return errors.New("max-output-bytes must be greater than zero")
	}
	if cfg.ConversionTimeout <= 0 {
		return errors.New("conversion-timeout must be greater than zero")
	}

	rendererMode := cfg.RendererMode
	if rendererMode == "" {
		rendererMode = render.ModeAuto
	}
	selectedRenderer, err := render.Select(rendererMode, cfg.Env, cfg.Force, nil)
	if err != nil {
		return err
	}

	resolve := cfg.Resolve
	if resolve == nil {
		resolve = source.Resolve
	}
	resolved, err := resolve(ctx, cfg.Args[0], cfg.MaxBytes)
	if err != nil {
		return err
	}
	defer resolved.Cleanup()

	img, err := imageio.Load(ctx, resolved.Path, imageio.Options{
		MaxPixels:         cfg.MaxPixels,
		MaxOutputBytes:    cfg.MaxOutputBytes,
		ConversionTimeout: cfg.ConversionTimeout,
	})
	if err != nil {
		return err
	}

	querySize := cfg.QuerySize
	if querySize == nil {
		querySize = terminal.QuerySize
	}
	size, err := querySize(cfg.TTYPath)
	if err != nil {
		return fmt.Errorf("query terminal size: %w", err)
	}

	placement, err := terminal.FitToViewport(img.Width, img.Height, size, cfg.ScaleUp)
	if err != nil {
		return err
	}

	if cfg.Stderr != nil &&
		cfg.Force &&
		rendererMode == render.ModeAuto &&
		selectedRenderer.Name() == "kitty" {
		_, _ = fmt.Fprintln(cfg.Stderr, "iview: warning: forcing Kitty graphics output for an unrecognized terminal")
	}

	if err := selectedRenderer.Render(cfg.Stdout, img, placement); err != nil {
		return fmt.Errorf("render image: %w", err)
	}
	return nil
}

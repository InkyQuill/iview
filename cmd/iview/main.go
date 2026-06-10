package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/InkyQuill/iview/internal/app"
	"github.com/InkyQuill/iview/internal/render"
)

const version = "0.1.0"

func main() {
	cfg := app.Config{
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		Env:               os.Environ(),
		TTYPath:           "/dev/tty",
		MaxBytes:          50 << 20,
		MaxPixels:         100_000_000,
		MaxOutputBytes:    100 << 20,
		ConversionTimeout: 5 * time.Second,
		RendererMode:      render.ModeAuto,
		ScaleUp:           false,
		Force:             false,
		ShowUsage:         nil,
	}

	flags := flag.NewFlagSet("iview", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	rendererValue := flags.String("renderer", "auto", "renderer to use: auto, kitty, iterm2, sixel, cells")
	flags.Int64Var(&cfg.MaxBytes, "max-bytes", cfg.MaxBytes, "maximum downloaded image size in bytes")
	flags.IntVar(&cfg.MaxPixels, "max-pixels", cfg.MaxPixels, "maximum decoded image pixels")
	flags.Int64Var(
		&cfg.MaxOutputBytes,
		"max-output-bytes",
		cfg.MaxOutputBytes,
		"maximum normalized image size in bytes",
	)
	flags.DurationVar(
		&cfg.ConversionTimeout,
		"conversion-timeout",
		cfg.ConversionTimeout,
		"maximum time spent in one external conversion",
	)
	flags.BoolVar(&cfg.ScaleUp, "scale-up", cfg.ScaleUp, "scale smaller images up to fill the viewport")
	flags.BoolVar(
		&cfg.Force,
		"force",
		cfg.Force,
		"force the selected renderer; with --renderer=auto, force backward-compatible Kitty output",
	)
	showVersion := flags.Bool("version", false, "print version and exit")

	flags.Usage = func() {
		_, _ = fmt.Fprintln(os.Stderr, "usage: iview [flags] <image-path-or-url>")
		flags.PrintDefaults()
	}
	cfg.ShowUsage = flags.Usage

	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if *showVersion {
		_, _ = fmt.Fprintf(os.Stdout, "iview %s\n", version)
		return
	}

	rendererMode, err := render.ParseMode(*rendererValue)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "iview: %v\n", err)
		os.Exit(2)
	}
	cfg.RendererMode = rendererMode

	cfg.Args = flags.Args()
	if err := app.Run(context.Background(), cfg); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "iview: %v\n", err)
		os.Exit(1)
	}
}

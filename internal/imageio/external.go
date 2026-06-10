package imageio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxConverterStderrBytes = 4096

type externalConverter struct {
	name string
	args func(input, output string) []string
}

func loadWithExternalConverter(ctx context.Context, path string, format Format, options Options) (NormalizedImage, error) {
	if format == FormatSVG {
		if err := validateSVGStaticSafety(path); err != nil {
			return NormalizedImage{}, err
		}
	}

	converter, exe, err := selectExternalConverter(format, options)
	if err != nil {
		return NormalizedImage{}, err
	}

	inputPath, err := filepath.Abs(path)
	if err != nil {
		return NormalizedImage{}, fmt.Errorf("resolve image path: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "iview-imageio-*")
	if err != nil {
		return NormalizedImage{}, fmt.Errorf("create conversion temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	outputPath := filepath.Join(tmpDir, "converted.png")
	convertCtx, cancel := context.WithTimeout(ctx, options.ConversionTimeout)
	defer cancel()

	stderr := &boundedStderr{limit: maxConverterStderrBytes}
	if err := options.CommandRunner.Run(convertCtx, exe, converter.args(inputPath, outputPath), io.Discard, stderr); err != nil {
		return NormalizedImage{}, conversionError(convertCtx, converter.name, err, stderr.String())
	}

	pngBytes, width, height, err := readConvertedPNG(outputPath, options)
	if err != nil {
		return NormalizedImage{}, err
	}

	return NormalizedImage{
		Width:        width,
		Height:       height,
		PNG:          pngBytes,
		SourceFormat: format,
	}, nil
}

func selectExternalConverter(format Format, options Options) (externalConverter, string, error) {
	for _, converter := range convertersForFormat(format) {
		exe, err := options.LookupExecutable(converter.name)
		if err == nil {
			return converter, exe, nil
		}
	}

	switch format {
	case FormatAVIF:
		return externalConverter{}, "", errors.New("AVIF support requires ffmpeg. Install ffmpeg or convert the image to PNG/JPEG")
	case FormatSVG:
		return externalConverter{}, "", errors.New("SVG support requires rsvg-convert. Install librsvg or convert the image to PNG")
	case FormatICO:
		return externalConverter{}, "", errors.New("ICO support requires ffmpeg. Install ffmpeg or convert the image to PNG")
	default:
		return externalConverter{}, "", fmt.Errorf("unsupported format %q", format)
	}
}

func convertersForFormat(format Format) []externalConverter {
	ffmpeg := externalConverter{
		name: "ffmpeg",
		args: func(input, output string) []string {
			return []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", input, "-frames:v", "1", output}
		},
	}
	magick := externalConverter{
		name: "magick",
		args: func(input, output string) []string {
			return []string{input + "[0]", "PNG32:" + output}
		},
	}
	rsvg := externalConverter{
		name: "rsvg-convert",
		args: func(input, output string) []string {
			return []string{"--format=png", "--keep-aspect-ratio", "--output", output, input}
		},
	}

	switch format {
	case FormatAVIF, FormatICO:
		return []externalConverter{ffmpeg, magick}
	case FormatSVG:
		return []externalConverter{rsvg}
	default:
		return nil
	}
}

func conversionError(ctx context.Context, tool string, err error, stderr string) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("image conversion with %s timed out: %w", tool, ctx.Err())
	}
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return fmt.Errorf("image conversion with %s failed: %w", tool, err)
	}
	return fmt.Errorf("image conversion with %s failed: %w: %s", tool, err, stderr)
}

type boundedStderr struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedStderr) Write(p []byte) (int, error) {
	remaining := b.limit - b.Len()
	if remaining > 0 {
		if len(p) <= remaining {
			_, _ = b.Buffer.Write(p)
		} else {
			_, _ = b.Buffer.Write(p[:remaining])
			b.truncated = true
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return len(p), nil
}

func (b *boundedStderr) String() string {
	s := b.Buffer.String()
	if b.truncated {
		return s + "... [stderr truncated]"
	}
	return s
}

func readConvertedPNG(path string, options Options) ([]byte, int, int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read converted png: %w", err)
	}
	if info.Size() > options.MaxOutputBytes {
		return nil, 0, 0, fmt.Errorf("normalized PNG exceeds limit %d", options.MaxOutputBytes)
	}

	pngBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("read converted png: %w", err)
	}
	decoded, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("external converter produced invalid png: %w", err)
	}
	bounds := decoded.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if err := validateDimensions(width, height, options.MaxPixels); err != nil {
		return nil, 0, 0, err
	}

	return pngBytes, width, height, nil
}

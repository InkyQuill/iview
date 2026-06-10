package imageio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	"golang.org/x/image/webp"
)

const (
	sniffBytes            = 512
	defaultMaxPixels      = 100_000_000
	defaultMaxOutputBytes = 100 << 20
	defaultConvertTimeout = 10 * time.Second
)

type Options struct {
	MaxPixels         int
	MaxOutputBytes    int64
	ConversionTimeout time.Duration
	LookupExecutable  func(string) (string, error)
	CommandRunner     CommandRunner
}

func (o Options) withDefaults() Options {
	if o.MaxPixels <= 0 {
		o.MaxPixels = defaultMaxPixels
	}
	if o.MaxOutputBytes <= 0 {
		o.MaxOutputBytes = defaultMaxOutputBytes
	}
	if o.ConversionTimeout == 0 {
		o.ConversionTimeout = defaultConvertTimeout
	}
	if o.LookupExecutable == nil {
		o.LookupExecutable = exec.LookPath
	}
	if o.CommandRunner == nil {
		o.CommandRunner = osCommandRunner{}
	}
	return o
}

type CommandRunner interface {
	Run(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error
}

type osCommandRunner struct{}

func (osCommandRunner) Run(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

type NormalizedImage struct {
	Width        int
	Height       int
	PNG          []byte
	SourceFormat Format
}

type Image = NormalizedImage

func Load(ctx context.Context, path string, options Options) (NormalizedImage, error) {
	_ = ctx
	options = options.withDefaults()

	f, err := os.Open(path)
	if err != nil {
		return NormalizedImage{}, fmt.Errorf("open image: %w", err)
	}
	defer func() { _ = f.Close() }()

	head := make([]byte, sniffBytes)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return NormalizedImage{}, fmt.Errorf("read image header: %w", err)
	}
	head = head[:n]

	format := detectFormat(head)
	if format == FormatUnknown {
		mime := http.DetectContentType(head)
		return NormalizedImage{}, fmt.Errorf("not an image: detected %s", mime)
	}

	if !isGoDecodedFormat(format) {
		return loadWithExternalConverter(ctx, path, format, options)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return NormalizedImage{}, fmt.Errorf("rewind image: %w", err)
	}
	return loadWithGoDecoder(f, format, options)
}

func isGoDecodedFormat(format Format) bool {
	switch format {
	case FormatPNG, FormatJPEG, FormatGIF, FormatWebP, FormatBMP, FormatTIFF:
		return true
	default:
		return false
	}
}

func loadWithGoDecoder(f *os.File, format Format, options Options) (NormalizedImage, error) {
	cfg, err := decodeConfig(f, format)
	if err != nil {
		return NormalizedImage{}, fmt.Errorf("unsupported or invalid image: %w", err)
	}
	if err := validateDimensions(cfg.Width, cfg.Height, options.MaxPixels); err != nil {
		return NormalizedImage{}, err
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return NormalizedImage{}, fmt.Errorf("rewind image: %w", err)
	}
	decoded, err := decode(f, format)
	if err != nil {
		return NormalizedImage{}, fmt.Errorf("decode image: %w", err)
	}

	pngBytes, err := encodeBoundedPNG(decoded, options.MaxOutputBytes)
	if err != nil {
		return NormalizedImage{}, err
	}

	return NormalizedImage{
		Width:        cfg.Width,
		Height:       cfg.Height,
		PNG:          pngBytes,
		SourceFormat: format,
	}, nil
}

func validateDimensions(width, height int, maxPixels int) error {
	if width <= 0 || height <= 0 {
		return errors.New("unsupported or invalid image: image dimensions must be positive")
	}
	if width > maxPixels/height {
		return fmt.Errorf("image dimensions %dx%d exceed %d pixels", width, height, maxPixels)
	}
	return nil
}

func encodeBoundedPNG(img image.Image, maxOutputBytes int64) ([]byte, error) {
	out := &limitedBuffer{
		Buffer: &bytes.Buffer{},
		Limit:  maxOutputBytes,
	}
	if err := png.Encode(out, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}

	return out.Bytes(), nil
}

type limitedBuffer struct {
	*bytes.Buffer
	Limit int64
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if int64(b.Len()+len(p)) > b.Limit {
		return 0, fmt.Errorf("normalized PNG exceeds limit %d", b.Limit)
	}
	return b.Buffer.Write(p)
}

func decodeConfig(r io.Reader, format Format) (image.Config, error) {
	switch format {
	case FormatPNG:
		cfg, err := png.DecodeConfig(r)
		return cfg, err
	case FormatJPEG:
		cfg, err := jpeg.DecodeConfig(r)
		return cfg, err
	case FormatGIF:
		cfg, err := gif.DecodeConfig(r)
		return cfg, err
	case FormatWebP:
		cfg, err := webp.DecodeConfig(r)
		return cfg, err
	case FormatBMP:
		cfg, err := bmp.DecodeConfig(r)
		return cfg, err
	case FormatTIFF:
		cfg, err := tiff.DecodeConfig(r)
		return cfg, err
	default:
		return image.Config{}, fmt.Errorf("unsupported format")
	}
}

func decode(r io.Reader, format Format) (image.Image, error) {
	switch format {
	case FormatPNG:
		return png.Decode(r)
	case FormatJPEG:
		return jpeg.Decode(r)
	case FormatGIF:
		return gif.Decode(r)
	case FormatWebP:
		return webp.Decode(r)
	case FormatBMP:
		return bmp.Decode(r)
	case FormatTIFF:
		return tiff.Decode(r)
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

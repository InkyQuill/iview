# Terminal Image Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add safe broad image normalization and renderer selection for GIF, BMP, JPEG, PNG, WebP, AVIF, SVG, ICO, TIFF, Kitty, iTerm2, Sixel, and terminal-cell fallback.

**Architecture:** Keep the current three-stage CLI flow: resolve source, normalize to bounded PNG, render with the best terminal backend. Add a small image normalization API with Go decoders and optional external adapters, then replace hard-coded Kitty output with renderer selection. External tools run through fixed `exec.CommandContext` calls, private temp dirs, timeouts, and output validation.

**Tech Stack:** Go, standard `image` decoders, `golang.org/x/image`, optional local tools (`ffmpeg`, `rsvg-convert`, `magick`, `img2sixel`), package-level Go tests.

---

## File Structure

- Modify `cmd/iview/main.go`: add renderer and safety flags, pass options to `app.Config`.
- Modify `internal/app/app.go`: wire source resolution, normalization options, renderer selection, cleanup, and force behavior.
- Modify `internal/app/app_test.go`: cover app flow, cleanup, renderer selection, missing tools, and no output before validation.
- Create `internal/imageio/format.go`: format enum, magic-byte/XML detection, install-hint errors.
- Modify `internal/imageio/imageio.go`: expose `NormalizedImage`, `Options`, and `Load(ctx, path, options)` normalization flow.
- Create `internal/imageio/external.go`: safe external command runner for conversion adapters.
- Create `internal/imageio/svg.go`: SVG static-safety checks.
- Modify `internal/imageio/imageio_test.go`: format detection, limits, Go decode behavior, fake external tools.
- Modify `internal/render/kitty.go`: adapt Kitty renderer to a common renderer interface.
- Create `internal/render/renderer.go`: renderer interface, renderer mode parsing, selector, placement helpers.
- Create `internal/render/iterm2.go`: iTerm2 inline-image renderer.
- Create `internal/render/sixel.go`: Sixel renderer backed by `img2sixel` when available.
- Create `internal/render/cells.go`: built-in Unicode half-block fallback renderer from normalized PNG bytes.
- Modify `internal/render/kitty_test.go`: adjust to new renderer API while preserving protocol golden checks.
- Create `internal/render/renderer_test.go`: renderer selection and mode parsing.
- Create `internal/render/iterm2_test.go`, `internal/render/sixel_test.go`, `internal/render/cells_test.go`: renderer output tests.
- Modify `internal/terminal/terminal.go`: keep viewport sizing; move graphics support detection into `internal/render`.
- Modify `internal/terminal/terminal_test.go`: keep viewport tests and remove Kitty support tests after replacement.
- Modify `README.md`: document formats, renderers, tools, flags, and security posture.

---

### Task 1: Image Format Detection And Limits

**Files:**
- Create: `internal/imageio/format.go`
- Modify: `internal/imageio/imageio.go`
- Test: `internal/imageio/imageio_test.go`

- [ ] **Step 1: Write failing format detection tests**

Add these tests to `internal/imageio/imageio_test.go`:

```go
func TestDetectFormatRecognizesRenderableFormats(t *testing.T) {
	tests := []struct {
		name string
		head []byte
		want Format
	}{
		{name: "png", head: []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, want: FormatPNG},
		{name: "jpeg", head: []byte{0xff, 0xd8, 0xff, 0xdb}, want: FormatJPEG},
		{name: "gif87a", head: []byte("GIF87a123"), want: FormatGIF},
		{name: "gif89a", head: []byte("GIF89a123"), want: FormatGIF},
		{name: "webp", head: []byte("RIFFxxxxWEBPVP8 "), want: FormatWebP},
		{name: "bmp", head: []byte{'B', 'M', 0, 0}, want: FormatBMP},
		{name: "tiff-le", head: []byte{'I', 'I', 42, 0}, want: FormatTIFF},
		{name: "tiff-be", head: []byte{'M', 'M', 0, 42}, want: FormatTIFF},
		{name: "avif", head: []byte{0, 0, 0, 32, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, want: FormatAVIF},
		{name: "avif-sequence", head: []byte{0, 0, 0, 32, 'f', 't', 'y', 'p', 'a', 'v', 'i', 's'}, want: FormatAVIF},
		{name: "ico", head: []byte{0, 0, 1, 0, 1, 0, 16, 16}, want: FormatICO},
		{name: "svg-xml", head: []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`), want: FormatSVG},
		{name: "svg-leading-space", head: []byte(" \n\t<svg viewBox=\"0 0 1 1\"></svg>"), want: FormatSVG},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectFormat(tt.head); got != tt.want {
				t.Fatalf("detectFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectFormatRejectsUnknownBytes(t *testing.T) {
	if got := detectFormat([]byte("plain text")); got != FormatUnknown {
		t.Fatalf("detectFormat() = %q, want unknown", got)
	}
}
```

- [ ] **Step 2: Run detection tests and verify failure**

Run:

```sh
go test ./internal/imageio -run 'TestDetectFormat' -count=1
```

Expected: compile failure because `Format`, `FormatPNG`, and the other constants do not exist yet.

- [ ] **Step 3: Add format enum and detection**

Create `internal/imageio/format.go`:

```go
package imageio

import (
	"bytes"
	"strings"
)

type Format string

const (
	FormatUnknown Format = ""
	FormatPNG     Format = "png"
	FormatJPEG    Format = "jpeg"
	FormatGIF     Format = "gif"
	FormatWebP    Format = "webp"
	FormatBMP     Format = "bmp"
	FormatTIFF    Format = "tiff"
	FormatAVIF    Format = "avif"
	FormatSVG     Format = "svg"
	FormatICO     Format = "ico"
)

func detectFormat(head []byte) Format {
	switch {
	case len(head) >= 8 && bytes.Equal(head[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}):
		return FormatPNG
	case len(head) >= 3 && bytes.Equal(head[:3], []byte{0xff, 0xd8, 0xff}):
		return FormatJPEG
	case len(head) >= 6 && (bytes.Equal(head[:6], []byte("GIF87a")) || bytes.Equal(head[:6], []byte("GIF89a"))):
		return FormatGIF
	case len(head) >= 12 && bytes.Equal(head[:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP")):
		return FormatWebP
	case len(head) >= 2 && bytes.Equal(head[:2], []byte("BM")):
		return FormatBMP
	case len(head) >= 4 && (bytes.Equal(head[:4], []byte{'I', 'I', 42, 0}) || bytes.Equal(head[:4], []byte{'M', 'M', 0, 42})):
		return FormatTIFF
	case isAVIF(head):
		return FormatAVIF
	case len(head) >= 4 && bytes.Equal(head[:4], []byte{0, 0, 1, 0}):
		return FormatICO
	case isSVG(head):
		return FormatSVG
	default:
		return FormatUnknown
	}
}

func isAVIF(head []byte) bool {
	if len(head) < 12 || !bytes.Equal(head[4:8], []byte("ftyp")) {
		return false
	}
	brands := string(head[8:])
	return strings.Contains(brands, "avif") || strings.Contains(brands, "avis")
}

func isSVG(head []byte) bool {
	trimmed := strings.TrimLeft(string(head), "\ufeff \t\r\n")
	if strings.HasPrefix(trimmed, "<?xml") {
		if end := strings.Index(trimmed, "?>"); end >= 0 {
			trimmed = strings.TrimLeft(trimmed[end+2:], " \t\r\n")
		}
	}
	return strings.HasPrefix(strings.ToLower(trimmed), "<svg")
}
```

Remove the old `detectFormat` function from `internal/imageio/imageio.go`.

- [ ] **Step 4: Run detection tests and verify pass**

Run:

```sh
go test ./internal/imageio -run 'TestDetectFormat' -count=1
```

Expected: PASS.

- [ ] **Step 5: Add option and normalized image types**

Modify the top of `internal/imageio/imageio.go`:

```go
const (
	sniffBytes            = 512
	defaultMaxPixels     = 100_000_000
	defaultMaxOutputBytes = 100 << 20
)

type Options struct {
	MaxPixels            int
	MaxOutputBytes       int64
	ConversionTimeout    time.Duration
	LookupExecutable     func(string) (string, error)
	CommandRunner         CommandRunner
}

type NormalizedImage struct {
	Width        int
	Height       int
	PNG          []byte
	SourceFormat Format
}

type Image = NormalizedImage

func (o Options) withDefaults() Options {
	if o.MaxPixels <= 0 {
		o.MaxPixels = defaultMaxPixels
	}
	if o.MaxOutputBytes <= 0 {
		o.MaxOutputBytes = defaultMaxOutputBytes
	}
	if o.ConversionTimeout <= 0 {
		o.ConversionTimeout = 5 * time.Second
	}
	if o.LookupExecutable == nil {
		o.LookupExecutable = exec.LookPath
	}
	if o.CommandRunner == nil {
		o.CommandRunner = osCommandRunner{}
	}
	return o
}
```

Add imports for `context`, `os/exec`, and `time`. Keep `type Image = NormalizedImage` so current app tests keep compiling during the transition.

- [ ] **Step 6: Commit detection and options**

Run:

```sh
git status --short
git add internal/imageio/format.go internal/imageio/imageio.go internal/imageio/imageio_test.go
git commit -m "feat: detect renderable image formats"
```

Expected if Git metadata is restored: commit succeeds. If `git status` reports `fatal: not a git repository`, record that commits are blocked by invalid `.git` metadata and continue without committing.

---

### Task 2: Normalization Flow With Go Decoders

**Files:**
- Modify: `internal/imageio/imageio.go`
- Test: `internal/imageio/imageio_test.go`

- [ ] **Step 1: Write failing normalization tests**

Add these tests to `internal/imageio/imageio_test.go`:

```go
func TestLoadRecordsSourceFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode test image: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close test image: %v", err)
	}

	loaded, err := Load(context.Background(), path, Options{})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.SourceFormat != FormatPNG {
		t.Fatalf("SourceFormat = %q, want %q", loaded.SourceFormat, FormatPNG)
	}
	if loaded.Width != 2 || loaded.Height != 3 {
		t.Fatalf("unexpected dimensions: %dx%d", loaded.Width, loaded.Height)
	}
	if len(loaded.PNG) == 0 {
		t.Fatal("expected PNG payload")
	}
}

func TestLoadRejectsPixelLimitBeforeDecode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode test image: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close test image: %v", err)
	}

	_, err = Load(context.Background(), path, Options{MaxPixels: 15})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "exceed 15 pixels") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsOversizedPNGOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode test image: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close test image: %v", err)
	}

	_, err = Load(context.Background(), path, Options{MaxOutputBytes: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "normalized PNG exceeds limit 1") {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

Update existing tests to call `Load(context.Background(), path, Options{})`.

- [ ] **Step 2: Run normalization tests and verify failure**

Run:

```sh
go test ./internal/imageio -count=1
```

Expected: compile failure because `Load` still has the old signature.

- [ ] **Step 3: Update Load and Go decoder helpers**

Modify `internal/imageio/imageio.go` so the public load function has this shape:

```go
func Load(ctx context.Context, path string, options Options) (NormalizedImage, error) {
	_ = ctx
	options = options.withDefaults()

	f, err := os.Open(path)
	if err != nil {
		return NormalizedImage{}, fmt.Errorf("open image: %w", err)
	}
	defer f.Close()

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

	if isGoDecodedFormat(format) {
		return loadWithGoDecoder(f, format, options)
	}
	return loadWithExternalConverter(ctx, path, format, options)
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
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return NormalizedImage{}, fmt.Errorf("rewind image: %w", err)
	}
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
	var out bytes.Buffer
	limited := &limitedBuffer{Buffer: &out, Limit: maxOutputBytes}
	if err := png.Encode(limited, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return out.Bytes(), nil
}

type limitedBuffer struct {
	*bytes.Buffer
	Limit int64
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if int64(b.Buffer.Len()+len(p)) > b.Limit {
		return 0, fmt.Errorf("normalized PNG exceeds limit %d", b.Limit)
	}
	return b.Buffer.Write(p)
}
```

Change `decodeConfig` and `decode` signatures from `format string` to `format Format`.

- [ ] **Step 4: Run imageio tests**

Run:

```sh
go test ./internal/imageio -count=1
```

Expected: failure mentioning `CommandRunner`, `osCommandRunner`, or `loadWithExternalConverter` until the next task creates the external adapter scaffolding. If those are already stubbed, expected: PASS.

- [ ] **Step 5: Add temporary external converter stub if needed**

If compilation needs the external hook before Task 3, add this temporary function at the bottom of `internal/imageio/imageio.go`:

```go
func loadWithExternalConverter(context.Context, string, Format, Options) (NormalizedImage, error) {
	return NormalizedImage{}, fmt.Errorf("unsupported format")
}
```

This stub must be replaced in Task 3.

- [ ] **Step 6: Commit Go normalization**

Run:

```sh
git status --short
git add internal/imageio/imageio.go internal/imageio/imageio_test.go
git commit -m "feat: normalize go-decoded images safely"
```

Expected if Git metadata is restored: commit succeeds. If `git status` reports `fatal: not a git repository`, record that commits are blocked by invalid `.git` metadata and continue without committing.

---

### Task 3: Safe External Conversion For AVIF, SVG, And ICO

**Files:**
- Create: `internal/imageio/external.go`
- Create: `internal/imageio/svg.go`
- Modify: `internal/imageio/imageio.go`
- Test: `internal/imageio/imageio_test.go`

- [ ] **Step 1: Write failing external conversion tests**

Add these helpers and tests to `internal/imageio/imageio_test.go`:

```go
type fakeRunner struct {
	calls []fakeCall
	run   func(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error
}

type fakeCall struct {
	exe  string
	args []string
}

func (r *fakeRunner) Run(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error {
	r.calls = append(r.calls, fakeCall{exe: exe, args: append([]string(nil), args...)})
	if r.run != nil {
		return r.run(ctx, exe, args, stdout, stderr)
	}
	return nil
}

func writeMinimalPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create png: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close png: %v", err)
	}
}

func TestLoadAVIFUsesFFmpegWhenAvailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.avif")
	if err := os.WriteFile(path, []byte{0, 0, 0, 32, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, 0o600); err != nil {
		t.Fatalf("write avif: %v", err)
	}
	runner := &fakeRunner{run: func(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error {
		out := args[len(args)-1]
		writeMinimalPNG(t, out)
		return nil
	}}

	img, err := Load(context.Background(), path, Options{
		LookupExecutable: func(name string) (string, error) {
			if name == "ffmpeg" {
				return "/usr/bin/ffmpeg", nil
			}
			return "", exec.ErrNotFound
		},
		CommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if img.SourceFormat != FormatAVIF {
		t.Fatalf("SourceFormat = %q, want avif", img.SourceFormat)
	}
	if len(runner.calls) != 1 || runner.calls[0].exe != "/usr/bin/ffmpeg" {
		t.Fatalf("unexpected calls: %+v", runner.calls)
	}
	if strings.Contains(strings.Join(runner.calls[0].args, " "), ";") {
		t.Fatalf("args contain shell separator: %+v", runner.calls[0].args)
	}
}

func TestLoadAVIFMissingToolHasInstallHint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.avif")
	if err := os.WriteFile(path, []byte{0, 0, 0, 32, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, 0o600); err != nil {
		t.Fatalf("write avif: %v", err)
	}

	_, err := Load(context.Background(), path, Options{
		LookupExecutable: func(string) (string, error) { return "", exec.ErrNotFound },
		CommandRunner:    &fakeRunner{},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "AVIF support requires ffmpeg") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadSVGRejectsExternalReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.svg")
	content := `<svg xmlns="http://www.w3.org/2000/svg"><image href="file:///etc/passwd"/></svg>`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write svg: %v", err)
	}

	_, err := Load(context.Background(), path, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "svg contains external resource reference") {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 2: Run external conversion tests and verify failure**

Run:

```sh
go test ./internal/imageio -run 'TestLoadAVIF|TestLoadSVG' -count=1
```

Expected: compile failure because `CommandRunner` and external conversion are not implemented, or test failure from the temporary unsupported-format stub.

- [ ] **Step 3: Add external command runner**

Create `internal/imageio/external.go`:

```go
package imageio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

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

func loadWithExternalConverter(ctx context.Context, path string, format Format, options Options) (NormalizedImage, error) {
	options = options.withDefaults()
	if format == FormatSVG {
		if err := validateStaticSVG(path, options.MaxOutputBytes); err != nil {
			return NormalizedImage{}, err
		}
	}

	adapter, err := chooseAdapter(format, options)
	if err != nil {
		return NormalizedImage{}, err
	}

	tmpDir, err := os.MkdirTemp("", "iview-convert-*")
	if err != nil {
		return NormalizedImage{}, fmt.Errorf("create conversion temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	outPath := filepath.Join(tmpDir, "out.png")
	convertCtx, cancel := context.WithTimeout(ctx, options.ConversionTimeout)
	defer cancel()

	var stderr bytes.Buffer
	if err := options.CommandRunner.Run(convertCtx, adapter.exe, adapter.args(path, outPath), io.Discard, &stderr); err != nil {
		if convertCtx.Err() != nil {
			return NormalizedImage{}, fmt.Errorf("conversion timed out after %s", options.ConversionTimeout)
		}
		return NormalizedImage{}, fmt.Errorf("%s conversion failed: %w: %s", format, err, summarizeStderr(stderr.String()))
	}

	return loadConvertedPNG(outPath, format, options)
}

type externalAdapter struct {
	exe  string
	args func(input, output string) []string
}

func chooseAdapter(format Format, options Options) (externalAdapter, error) {
	switch format {
	case FormatAVIF:
		if exe, err := options.LookupExecutable("ffmpeg"); err == nil {
			return externalAdapter{exe: exe, args: ffmpegPNGArgs}, nil
		}
		if exe, err := options.LookupExecutable("magick"); err == nil {
			return externalAdapter{exe: exe, args: magickPNGArgs}, nil
		}
		return externalAdapter{}, fmt.Errorf("AVIF support requires ffmpeg. Install ffmpeg or convert the image to PNG/JPEG")
	case FormatSVG:
		if exe, err := options.LookupExecutable("rsvg-convert"); err == nil {
			return externalAdapter{exe: exe, args: rsvgPNGArgs}, nil
		}
		return externalAdapter{}, fmt.Errorf("SVG support requires rsvg-convert. Install librsvg or convert the image to PNG")
	case FormatICO:
		if exe, err := options.LookupExecutable("ffmpeg"); err == nil {
			return externalAdapter{exe: exe, args: ffmpegPNGArgs}, nil
		}
		if exe, err := options.LookupExecutable("magick"); err == nil {
			return externalAdapter{exe: exe, args: magickPNGArgs}, nil
		}
		return externalAdapter{}, fmt.Errorf("ICO support requires ffmpeg. Install ffmpeg or convert the image to PNG")
	default:
		return externalAdapter{}, fmt.Errorf("unsupported format %q", format)
	}
}

func ffmpegPNGArgs(input, output string) []string {
	return []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", input, "-frames:v", "1", output}
}

func magickPNGArgs(input, output string) []string {
	return []string{input + "[0]", "PNG32:" + output}
}

func rsvgPNGArgs(input, output string) []string {
	return []string{"--format=png", "--keep-aspect-ratio", "--output", output, input}
}

func loadConvertedPNG(path string, sourceFormat Format, options Options) (NormalizedImage, error) {
	info, err := os.Stat(path)
	if err != nil {
		return NormalizedImage{}, fmt.Errorf("stat converted png: %w", err)
	}
	if info.Size() > options.MaxOutputBytes {
		return NormalizedImage{}, fmt.Errorf("converted PNG exceeds limit %d", options.MaxOutputBytes)
	}

	f, err := os.Open(path)
	if err != nil {
		return NormalizedImage{}, fmt.Errorf("open converted png: %w", err)
	}
	defer f.Close()

	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return NormalizedImage{}, fmt.Errorf("converted output is not a valid PNG: %w", err)
	}
	if err := validateDimensions(cfg.Width, cfg.Height, options.MaxPixels); err != nil {
		return NormalizedImage{}, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return NormalizedImage{}, fmt.Errorf("rewind converted png: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(f, options.MaxOutputBytes+1))
	if err != nil {
		return NormalizedImage{}, fmt.Errorf("read converted png: %w", err)
	}
	if int64(len(data)) > options.MaxOutputBytes {
		return NormalizedImage{}, fmt.Errorf("converted PNG exceeds limit %d", options.MaxOutputBytes)
	}
	return NormalizedImage{Width: cfg.Width, Height: cfg.Height, PNG: data, SourceFormat: sourceFormat}, nil
}

func summarizeStderr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
```

Ensure imports include `image/png` and `strings`.

- [ ] **Step 4: Add SVG static-safety checks**

Create `internal/imageio/svg.go`:

```go
package imageio

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
)

var svgUnsafePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<\s*script\b`),
	regexp.MustCompile(`(?i)\bon[a-z]+\s*=`),
	regexp.MustCompile(`(?i)\b(?:href|xlink:href|src)\s*=\s*['"]\s*(?:https?:|file:|data:)`),
	regexp.MustCompile(`(?i)\b(?:href|xlink:href|src)\s*=\s*['"]\s*/`),
	regexp.MustCompile(`(?i)<!ENTITY\b`),
}

func validateStaticSVG(path string, maxRead int64) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open svg: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxRead+1))
	if err != nil {
		return fmt.Errorf("read svg: %w", err)
	}
	if int64(len(data)) > maxRead {
		return fmt.Errorf("svg exceeds scan limit %d", maxRead)
	}
	lower := bytes.ToLower(data)
	if !bytes.Contains(lower, []byte("<svg")) {
		return fmt.Errorf("invalid svg: missing svg element")
	}
	for _, pattern := range svgUnsafePatterns {
		if pattern.Match(data) {
			return fmt.Errorf("svg contains external resource reference or executable content")
		}
	}
	return nil
}
```

- [ ] **Step 5: Run external conversion tests**

Run:

```sh
go test ./internal/imageio -run 'TestLoadAVIF|TestLoadSVG' -count=1
```

Expected: PASS.

- [ ] **Step 6: Run all imageio tests**

Run:

```sh
go test ./internal/imageio -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit external conversion**

Run:

```sh
git status --short
git add internal/imageio/external.go internal/imageio/svg.go internal/imageio/imageio.go internal/imageio/imageio_test.go
git commit -m "feat: add safe external image conversion"
```

Expected if Git metadata is restored: commit succeeds. If `git status` reports `fatal: not a git repository`, record that commits are blocked by invalid `.git` metadata and continue without committing.

---

### Task 4: Renderer Interfaces And Terminal Backends

**Files:**
- Modify: `internal/render/kitty.go`
- Create: `internal/render/renderer.go`
- Create: `internal/render/iterm2.go`
- Create: `internal/render/sixel.go`
- Create: `internal/render/cells.go`
- Modify: `internal/render/kitty_test.go`
- Create: `internal/render/renderer_test.go`
- Create: `internal/render/iterm2_test.go`
- Create: `internal/render/sixel_test.go`
- Create: `internal/render/cells_test.go`

- [ ] **Step 1: Write renderer selection tests**

Create `internal/render/renderer_test.go`:

```go
package render

import "testing"

func TestParseMode(t *testing.T) {
	tests := map[string]Mode{
		"":       ModeAuto,
		"auto":   ModeAuto,
		"kitty":  ModeKitty,
		"iterm2": ModeITerm2,
		"sixel":  ModeSixel,
		"cells":  ModeCells,
	}
	for input, want := range tests {
		got, err := ParseMode(input)
		if err != nil {
			t.Fatalf("ParseMode(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSelectAutoChoosesKittyBeforeCells(t *testing.T) {
	renderer, err := Select(ModeAuto, []string{"KITTY_WINDOW_ID=1"}, false, nil)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if renderer.Name() != "kitty" {
		t.Fatalf("renderer = %q, want kitty", renderer.Name())
	}
}

func TestSelectAutoChoosesITerm2(t *testing.T) {
	renderer, err := Select(ModeAuto, []string{"TERM_PROGRAM=iTerm.app"}, false, nil)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if renderer.Name() != "iterm2" {
		t.Fatalf("renderer = %q, want iterm2", renderer.Name())
	}
}

func TestSelectAutoFallsBackToCells(t *testing.T) {
	renderer, err := Select(ModeAuto, []string{"TERM=xterm-256color"}, false, nil)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if renderer.Name() != "cells" {
		t.Fatalf("renderer = %q, want cells", renderer.Name())
	}
}

func TestSelectForceAutoKeepsKittyCompatibility(t *testing.T) {
	renderer, err := Select(ModeAuto, []string{"TERM=xterm-256color"}, true, nil)
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}
	if renderer.Name() != "kitty" {
		t.Fatalf("renderer = %q, want kitty", renderer.Name())
	}
}
```

- [ ] **Step 2: Run renderer selection tests and verify failure**

Run:

```sh
go test ./internal/render -run 'TestParseMode|TestSelect' -count=1
```

Expected: compile failure because `Mode`, `ParseMode`, and `Select` do not exist.

- [ ] **Step 3: Add renderer interface and selector**

Create `internal/render/renderer.go`:

```go
package render

import (
	"fmt"
	"io"
	"strings"

	"iview/internal/imageio"
	"iview/internal/terminal"
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
	if value == "" {
		return ModeAuto, nil
	}
	mode := Mode(strings.ToLower(value))
	switch mode {
	case ModeAuto, ModeKitty, ModeITerm2, ModeSixel, ModeCells:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported renderer %q", value)
	}
}

func Select(mode Mode, env []string, force bool, lookup ToolLookup) (Renderer, error) {
	if lookup == nil {
		lookup = defaultToolLookup
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
			if renderer.Name() == string(mode) {
				if force || renderer.Supported(env) {
					return renderer, nil
				}
				return nil, fmt.Errorf("renderer %q is not supported by this terminal; use --force to override detection", mode)
			}
		}
	}
	for _, renderer := range renderers {
		if renderer.Supported(env) {
			return renderer, nil
		}
	}
	return CellsRenderer{}, nil
}

func envMap(env []string) map[string]string {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

func defaultToolLookup(name string) (string, error) {
	return exec.LookPath(name)
}
```

Add import `os/exec`.

- [ ] **Step 4: Convert Kitty output into a renderer**

Modify `internal/render/kitty.go`:

```go
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

func (KittyRenderer) Name() string { return "kitty" }

func (KittyRenderer) Supported(env []string) bool {
	values := envMap(env)
	term := strings.ToLower(values["TERM"])
	termProgram := strings.ToLower(values["TERM_PROGRAM"])
	switch {
	case values["KITTY_WINDOW_ID"] != "":
		return true
	case values["GHOSTTY_RESOURCES_DIR"] != "" || termProgram == "ghostty":
		return true
	case values["WEZTERM_EXECUTABLE"] != "" || strings.Contains(termProgram, "wezterm"):
		return true
	case values["KONSOLE_VERSION"] != "":
		return true
	case strings.Contains(term, "kitty"):
		return true
	default:
		return false
	}
}

func (r KittyRenderer) Render(w io.Writer, img imageio.NormalizedImage, placement terminal.Placement) error {
	return KittyPNG(w, img.PNG, placement.Columns, placement.Rows)
}

func KittyPNG(w io.Writer, png []byte, columns, rows int) error {
	if len(png) == 0 {
		return fmt.Errorf("empty png payload")
	}
	if columns <= 0 || rows <= 0 {
		return fmt.Errorf("placement dimensions must be positive")
	}

	encoder := base64.StdEncoding
	rawChunk := maxEncodedChunk / 4 * 3
	for offset := 0; offset < len(png); offset += rawChunk {
		end := min(offset+rawChunk, len(png))
		chunk := png[offset:end]
		encoded := make([]byte, encoder.EncodedLen(len(chunk)))
		encoder.Encode(encoded, chunk)

		more := 0
		if end < len(png) {
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
	_, err := fmt.Fprint(w, "\n")
	return err
}
```

- [ ] **Step 5: Add iTerm2 renderer**

Create `internal/render/iterm2.go`:

```go
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

func (ITerm2Renderer) Name() string { return "iterm2" }

func (ITerm2Renderer) Supported(env []string) bool {
	values := envMap(env)
	return strings.EqualFold(values["TERM_PROGRAM"], "iTerm.app")
}

func (ITerm2Renderer) Render(w io.Writer, img imageio.NormalizedImage, placement terminal.Placement) error {
	if len(img.PNG) == 0 {
		return fmt.Errorf("empty png payload")
	}
	if placement.Columns <= 0 || placement.Rows <= 0 {
		return fmt.Errorf("placement dimensions must be positive")
	}
	encoded := base64.StdEncoding.EncodeToString(img.PNG)
	_, err := fmt.Fprintf(w, "\x1b]1337;File=inline=1;width=%d;height=%d;preserveAspectRatio=1:%s\a\n", placement.Columns, placement.Rows, encoded)
	return err
}
```

- [ ] **Step 6: Add cells renderer**

Create `internal/render/cells.go`:

```go
package render

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"

	"iview/internal/imageio"
	"iview/internal/terminal"
)

type CellsRenderer struct{}

func (CellsRenderer) Name() string { return "cells" }

func (CellsRenderer) Supported([]string) bool { return true }

func (CellsRenderer) Render(w io.Writer, img imageio.NormalizedImage, placement terminal.Placement) error {
	decoded, err := png.Decode(bytes.NewReader(img.PNG))
	if err != nil {
		return fmt.Errorf("decode normalized png for cells: %w", err)
	}
	if placement.Columns <= 0 || placement.Rows <= 0 {
		return fmt.Errorf("placement dimensions must be positive")
	}
	bounds := decoded.Bounds()
	for row := 0; row < placement.Rows; row++ {
		y := bounds.Min.Y + row*bounds.Dy()/placement.Rows
		for col := 0; col < placement.Columns; col++ {
			x := bounds.Min.X + col*bounds.Dx()/placement.Columns
			r, g, b, _ := decoded.At(x, y).RGBA()
			if _, err := fmt.Fprintf(w, "\x1b[38;2;%d;%d;%dm▀", uint8(r>>8), uint8(g>>8), uint8(b>>8)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(w, "\x1b[0m\n"); err != nil {
			return err
		}
	}
	return nil
}

var _ image.Image
```

Remove `var _ image.Image` and the `image` import if the compiler reports it as unused.

- [ ] **Step 7: Add Sixel renderer with tool detection**

Create `internal/render/sixel.go`:

```go
package render

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"iview/internal/imageio"
	"iview/internal/terminal"
)

type SixelRenderer struct {
	Lookup ToolLookup
}

func (r SixelRenderer) Name() string { return "sixel" }

func (r SixelRenderer) Supported(env []string) bool {
	values := envMap(env)
	if values["TERM"] == "xterm-sixel" || values["DEC_IMAGE_PROTOCOL"] == "sixel" || values["SIXEL"] == "1" {
		return r.hasTool()
	}
	return false
}

func (r SixelRenderer) Render(w io.Writer, img imageio.NormalizedImage, placement terminal.Placement) error {
	if !r.hasTool() {
		return fmt.Errorf("Sixel rendering requires img2sixel. Install libsixel or use --renderer=cells")
	}
	exe, _ := r.lookup()("img2sixel")
	tmp, err := os.CreateTemp("", "iview-sixel-*.png")
	if err != nil {
		return fmt.Errorf("create sixel temp image: %w", err)
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := tmp.Write(img.PNG); err != nil {
		tmp.Close()
		return fmt.Errorf("write sixel temp image: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close sixel temp image: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-w", fmt.Sprint(placement.Columns), path)
	var stderr bytes.Buffer
	cmd.Stdout = w
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("sixel conversion timed out")
		}
		return fmt.Errorf("sixel conversion failed: %w: %s", err, stderr.String())
	}
	return nil
}

func (r SixelRenderer) hasTool() bool {
	_, err := r.lookup()("img2sixel")
	return err == nil
}

func (r SixelRenderer) lookup() ToolLookup {
	if r.Lookup != nil {
		return r.Lookup
	}
	return defaultToolLookup
}
```

- [ ] **Step 8: Add renderer output tests**

Create `internal/render/iterm2_test.go`:

```go
package render

import (
	"bytes"
	"strings"
	"testing"

	"iview/internal/imageio"
	"iview/internal/terminal"
)

func TestITerm2RendererWritesInlineImage(t *testing.T) {
	var out bytes.Buffer
	err := ITerm2Renderer{}.Render(&out, imageio.NormalizedImage{PNG: []byte("abc")}, terminal.Placement{Columns: 2, Rows: 3})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "\x1b]1337;File=inline=1;width=2;height=3;preserveAspectRatio=1:YWJj") {
		t.Fatalf("unexpected output: %q", got)
	}
}
```

Create `internal/render/cells_test.go` with a helper that encodes a 1x1 PNG and verifies ANSI truecolor output:

```go
package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"iview/internal/imageio"
	"iview/internal/terminal"
)

func TestCellsRendererWritesANSIBlocks(t *testing.T) {
	var pngBytes bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	var out bytes.Buffer
	err := CellsRenderer{}.Render(&out, imageio.NormalizedImage{PNG: pngBytes.Bytes()}, terminal.Placement{Columns: 1, Rows: 1})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(out.String(), "\x1b[38;2;10;20;30m▀") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}
```

- [ ] **Step 9: Run render tests**

Run:

```sh
go test ./internal/render -count=1
```

Expected: PASS after resolving any import cleanup from the snippets above.

- [ ] **Step 10: Commit renderers**

Run:

```sh
git status --short
git add internal/render
git commit -m "feat: add terminal renderer selection"
```

Expected if Git metadata is restored: commit succeeds. If `git status` reports `fatal: not a git repository`, record that commits are blocked by invalid `.git` metadata and continue without committing.

---

### Task 5: App And CLI Wiring

**Files:**
- Modify: `cmd/iview/main.go`
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`
- Modify: `internal/terminal/terminal.go`
- Modify: `internal/terminal/terminal_test.go`

- [ ] **Step 1: Write failing app wiring tests**

Add tests to `internal/app/app_test.go`:

```go
func TestRunFallsBackToCellsForUnsupportedTerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	writeAppPNG(t, path)

	var stdout bytes.Buffer
	err := Run(context.Background(), Config{
		Args:     []string{path},
		Env:      []string{"TERM=xterm-256color"},
		Stdout:   &stdout,
		MaxBytes: 1024,
		QuerySize: func(string) (terminal.Size, error) {
			return terminal.Size{Rows: 10, Columns: 10, WidthPixels: 100, HeightPixels: 100}, nil
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if stdout.Len() == 0 {
		t.Fatal("expected fallback output")
	}
}

func TestRunForceAutoWarnsAndUsesKitty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	writeAppPNG(t, path)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := Run(context.Background(), Config{
		Args:     []string{path},
		Env:      []string{"TERM=xterm-256color"},
		Stdout:   &stdout,
		Stderr:   &stderr,
		MaxBytes: 1024,
		Force:    true,
		QuerySize: func(string) (terminal.Size, error) {
			return terminal.Size{Rows: 10, Columns: 10, WidthPixels: 100, HeightPixels: 100}, nil
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "\x1b_G") {
		t.Fatalf("expected kitty output, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "forcing Kitty graphics output") {
		t.Fatalf("expected force warning, got %q", stderr.String())
	}
}

func writeAppPNG(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode image: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close image: %v", err)
	}
}
```

Add imports for `image`, `image/color`, `image/png`, and `path/filepath`.

- [ ] **Step 2: Run app tests and verify failure**

Run:

```sh
go test ./internal/app -count=1
```

Expected: failure because app still rejects unsupported terminals and calls old image/render APIs.

- [ ] **Step 3: Update app config and flow**

Modify `internal/app/app.go`:

```go
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
	ScaleUp           bool
	Force             bool
	RendererMode      render.Mode
	ShowUsage         func()
	QuerySize         func(string) (terminal.Size, error)
	Resolve           func(context.Context, string, int64) (source.Resolved, error)
}
```

Update `Run` to remove `terminal.SupportsKittyGraphics` and use renderer selection:

```go
selectedRenderer, err := render.Select(cfg.RendererMode, cfg.Env, cfg.Force, nil)
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
```

After placement calculation, render with:

```go
if cfg.Stderr != nil && cfg.Force && cfg.RendererMode == render.ModeAuto && selectedRenderer.Name() == "kitty" {
	fmt.Fprintln(cfg.Stderr, "iview: warning: forcing Kitty graphics output for an unrecognized terminal")
}

if err := selectedRenderer.Render(cfg.Stdout, img, placement); err != nil {
	return fmt.Errorf("render image: %w", err)
}
```

Ensure imports include `time` and remove direct `os` usage from cleanup.

- [ ] **Step 4: Update CLI flags**

Modify `cmd/iview/main.go`:

```go
rendererValue := flags.String("renderer", "auto", "renderer to use: auto, kitty, iterm2, sixel, cells")
flags.IntVar(&cfg.MaxPixels, "max-pixels", 100_000_000, "maximum decoded image pixels")
flags.Int64Var(&cfg.MaxOutputBytes, "max-output-bytes", 100<<20, "maximum normalized image size in bytes")
flags.DurationVar(&cfg.ConversionTimeout, "conversion-timeout", 5*time.Second, "maximum time spent in one external conversion")
```

After flag parsing:

```go
mode, err := render.ParseMode(*rendererValue)
if err != nil {
	fmt.Fprintf(os.Stderr, "iview: %v\n", err)
	os.Exit(2)
}
cfg.RendererMode = mode
```

Add imports for `time` and `iview/internal/render`.

- [ ] **Step 5: Remove obsolete terminal support detection tests and code**

In `internal/terminal/terminal.go`, remove `SupportsKittyGraphics`, `UnsupportedError`, and `envMap` if no other package uses them. In `internal/terminal/terminal_test.go`, remove `TestSupportsKittyGraphicsRecognizesGhostty` and `TestUnsupportedErrorIncludesTerminalEnvironment`. Keep both viewport fit tests unchanged.

- [ ] **Step 6: Run app and terminal tests**

Run:

```sh
go test ./internal/app ./internal/terminal -count=1
```

Expected: PASS.

- [ ] **Step 7: Run all tests**

Run:

```sh
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit app wiring**

Run:

```sh
git status --short
git add cmd/iview/main.go internal/app/app.go internal/app/app_test.go internal/terminal/terminal.go internal/terminal/terminal_test.go
git commit -m "feat: wire renderer and normalization options"
```

Expected if Git metadata is restored: commit succeeds. If `git status` reports `fatal: not a git repository`, record that commits are blocked by invalid `.git` metadata and continue without committing.

---

### Task 6: Documentation And Final Verification

**Files:**
- Modify: `README.md`
- Test: full test suite and build command

- [ ] **Step 1: Update README supported formats**

Replace the current support paragraph with:

```markdown
Supported input formats are PNG, JPEG, GIF, WebP, BMP, TIFF, AVIF, SVG, and ICO.
GIF rendering uses the first frame only. SVG rendering is static-only: scripts,
animation, and external resource references are rejected.
```

- [ ] **Step 2: Update README renderer description**

Replace the terminal support paragraph with:

```markdown
By default, `iview` auto-selects a renderer. It prefers real terminal image
protocols in this order: Kitty graphics, iTerm2 inline images, and Sixel. If no
real image protocol is detected, it falls back to a terminal-cell approximation
using ANSI color and block characters.
```

- [ ] **Step 3: Update README flags**

Replace the flags block with:

```text
--max-bytes N           maximum downloaded image size, default 52428800
--max-pixels N          maximum decoded image pixels, default 100000000
--max-output-bytes N    maximum normalized image size, default 104857600
--conversion-timeout D  maximum time per external conversion, default 5s
--renderer NAME         renderer: auto, kitty, iterm2, sixel, cells
--scale-up              scale smaller images up to fill the viewport
--force                 bypass renderer detection; with auto, force Kitty output
--version               print version
```

- [ ] **Step 4: Add README optional tools section**

Add:

```markdown
## Optional tools

Some formats and renderers use external tools when installed:

- `ffmpeg`: AVIF and ICO conversion.
- `rsvg-convert`: static SVG rasterization.
- `magick`: fallback conversion for AVIF and ICO when available.
- `img2sixel`: Sixel output when native Sixel emission is not available.

External tools are invoked without a shell, with fixed arguments, timeouts,
private temporary files, and output validation before anything is rendered.
If a required tool is missing, `iview` prints an install hint.
```

- [ ] **Step 5: Add README safety section**

Add:

```markdown
## Safety

`iview` treats local and remote inputs as untrusted. It validates image headers,
limits downloaded bytes, limits decoded pixels, limits normalized output size,
and emits no terminal graphics until normalization succeeds. Remote downloads
are stored in temporary files and removed after rendering or failure.
```

- [ ] **Step 6: Run full test suite**

Run:

```sh
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 7: Build the CLI**

Run:

```sh
go build -o bin/iview ./cmd/iview
```

Expected: command exits successfully and updates `bin/iview`.

- [ ] **Step 8: Manual smoke checks with installed tools**

Run:

```sh
./bin/iview --renderer=cells README.md
```

Expected: error containing `not an image` and no escape-heavy image output.

Create a tiny smoke-test PNG:

```sh
printf '%s' 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGNgYPgPAAEDAQCqD5tFAAAAAElFTkSuQmCC' | base64 -d > /tmp/iview-smoke.png
./bin/iview --renderer=cells /tmp/iview-smoke.png
```

Expected: ANSI block output appears in the terminal.

- [ ] **Step 9: Commit documentation**

Run:

```sh
git status --short
git add README.md docs/superpowers/specs/2026-06-10-terminal-image-support-design.md docs/superpowers/plans/2026-06-10-terminal-image-support.md
git commit -m "docs: describe terminal image support plan"
```

Expected if Git metadata is restored: commit succeeds. If `git status` reports `fatal: not a git repository`, record that commits are blocked by invalid `.git` metadata and continue without committing.

---

## Self-Review

- Spec coverage: the tasks cover requested formats, static SVG policy, first-frame GIF, optional external tools, install hints, renderer selection, cell fallback, safety limits, cleanup centralization, and documentation.
- Scope: this is one implementation plan because all changes feed a single CLI pipeline and can be verified with package tests plus a final build.
- Type consistency: `imageio.NormalizedImage`, `imageio.Options`, `render.Mode`, `render.Renderer`, and `terminal.Placement` are used consistently across tasks.
- Commit caveat: commit steps are included, but execution is blocked until `/home/inky/Development/iview/.git` contains valid repository metadata.

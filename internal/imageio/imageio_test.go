package imageio

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadAcceptsPNGAndReturnsPNGBytes(t *testing.T) {
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
	if loaded.Width != 2 || loaded.Height != 3 {
		t.Fatalf("unexpected dimensions: %dx%d", loaded.Width, loaded.Height)
	}
	if len(loaded.PNG) == 0 {
		t.Fatal("expected PNG payload")
	}
}

func TestLoadRejectsNonImageMagicBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-image.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := Load(context.Background(), path, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not an image") {
		t.Fatalf("unexpected error: %v", err)
	}
}

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

func TestLoadNormalizesJPEGToPNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test image: %v", err)
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("encode test image: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close test image: %v", err)
	}

	loaded, err := Load(context.Background(), path, Options{})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.SourceFormat != FormatJPEG {
		t.Fatalf("SourceFormat = %q, want %q", loaded.SourceFormat, FormatJPEG)
	}
	if loaded.Width != 3 || loaded.Height != 2 {
		t.Fatalf("unexpected dimensions: %dx%d", loaded.Width, loaded.Height)
	}

	cfg, err := png.DecodeConfig(bytes.NewReader(loaded.PNG))
	if err != nil {
		t.Fatalf("normalized PNG did not decode: %v", err)
	}
	if cfg.Width != 3 || cfg.Height != 2 {
		t.Fatalf("unexpected normalized PNG dimensions: %dx%d", cfg.Width, cfg.Height)
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

func TestLoadAVIFUsesFFmpegWhenAvailable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.avif")
	if err := os.WriteFile(path, []byte{0, 0, 0, 32, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, 0o600); err != nil {
		t.Fatalf("write avif fixture: %v", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}

	runner := &fakeRunner{
		run: func(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("expected conversion context to have deadline")
			}
			writeMinimalPNG(t, args[len(args)-1])
			return nil
		},
	}

	loaded, err := Load(context.Background(), path, Options{
		ConversionTimeout: time.Second,
		LookupExecutable: func(name string) (string, error) {
			if name == "ffmpeg" {
				return "/test/bin/ffmpeg", nil
			}
			t.Fatalf("unexpected lookup for %q", name)
			return "", os.ErrNotExist
		},
		CommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.SourceFormat != FormatAVIF {
		t.Fatalf("SourceFormat = %q, want %q", loaded.SourceFormat, FormatAVIF)
	}
	if loaded.Width != 1 || loaded.Height != 1 {
		t.Fatalf("unexpected dimensions: %dx%d", loaded.Width, loaded.Height)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(runner.calls))
	}

	call := runner.calls[0]
	if call.exe != "/test/bin/ffmpeg" {
		t.Fatalf("runner exe = %q, want /test/bin/ffmpeg", call.exe)
	}
	wantArgs := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", absPath, "-frames:v", "1"}
	if len(call.args) != len(wantArgs)+1 {
		t.Fatalf("runner args = %#v", call.args)
	}
	for i, want := range wantArgs {
		if call.args[i] != want {
			t.Fatalf("runner args[%d] = %q, want %q; all args: %#v", i, call.args[i], want, call.args)
		}
	}
	if filepath.Dir(call.args[len(call.args)-1]) == dir {
		t.Fatalf("converter output was not placed in private temp dir: %#v", call.args)
	}
	for _, arg := range call.args {
		if arg == ";" || arg == "&&" || arg == "|" {
			t.Fatalf("runner args include shell separator %q: %#v", arg, call.args)
		}
	}
}

func TestExternalConverterUsesAbsoluteInputPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.avif")
	if err := os.WriteFile(path, []byte{0, 0, 0, 32, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, 0o600); err != nil {
		t.Fatalf("write avif fixture: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	relPath, err := filepath.Rel(wd, path)
	if err != nil {
		t.Fatalf("relative path: %v", err)
	}

	runner := &fakeRunner{
		run: func(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error {
			if !filepath.IsAbs(args[6]) {
				t.Fatalf("converter input path = %q, want absolute path", args[6])
			}
			writeMinimalPNG(t, args[len(args)-1])
			return nil
		},
	}

	_, err = Load(context.Background(), relPath, Options{
		LookupExecutable: func(name string) (string, error) {
			if name == "ffmpeg" {
				return "/test/bin/ffmpeg", nil
			}
			return "", os.ErrNotExist
		},
		CommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
}

func TestExternalConverterBoundsStderrOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.avif")
	if err := os.WriteFile(path, []byte{0, 0, 0, 32, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, 0o600); err != nil {
		t.Fatalf("write avif fixture: %v", err)
	}

	runner := &fakeRunner{
		run: func(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error {
			if _, err := stderr.Write(bytes.Repeat([]byte("x"), 16*1024)); err != nil {
				t.Fatalf("write stderr: %v", err)
			}
			return errors.New("converter failed")
		},
	}

	_, err := Load(context.Background(), path, Options{
		LookupExecutable: func(name string) (string, error) {
			if name == "ffmpeg" {
				return "/test/bin/ffmpeg", nil
			}
			return "", os.ErrNotExist
		},
		CommandRunner: runner,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if len(msg) > 4600 {
		t.Fatalf("error length = %d, want bounded error; error: %q", len(msg), msg)
	}
	if !strings.Contains(msg, "stderr truncated") {
		t.Fatalf("expected truncation marker in error: %v", err)
	}
}

func TestExternalConverterRejectsTruncatedPNGAfterHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.avif")
	if err := os.WriteFile(path, []byte{0, 0, 0, 32, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, 0o600); err != nil {
		t.Fatalf("write avif fixture: %v", err)
	}
	corruptPNG := truncatedPNGPassingDecodeConfig(t)

	runner := &fakeRunner{
		run: func(ctx context.Context, exe string, args []string, stdout, stderr io.Writer) error {
			if err := os.WriteFile(args[len(args)-1], corruptPNG, 0o600); err != nil {
				t.Fatalf("write corrupt png: %v", err)
			}
			return nil
		},
	}

	_, err := Load(context.Background(), path, Options{
		LookupExecutable: func(name string) (string, error) {
			if name == "ffmpeg" {
				return "/test/bin/ffmpeg", nil
			}
			return "", os.ErrNotExist
		},
		CommandRunner: runner,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "external converter produced invalid png") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(runner.calls))
	}
}

func TestLoadAVIFMissingToolReturnsInstallHint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.avif")
	if err := os.WriteFile(path, []byte{0, 0, 0, 32, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, 0o600); err != nil {
		t.Fatalf("write avif fixture: %v", err)
	}

	runner := &fakeRunner{}
	_, err := Load(context.Background(), path, Options{
		LookupExecutable: func(string) (string, error) {
			return "", os.ErrNotExist
		},
		CommandRunner: runner,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "AVIF support requires ffmpeg") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0", len(runner.calls))
	}
}

func TestLoadSVGRejectsExternalResourceReferences(t *testing.T) {
	tests := []struct {
		name string
		svg  string
	}{
		{name: "file href", svg: `<svg xmlns="http://www.w3.org/2000/svg"><image href="file:///etc/passwd"/></svg>`},
		{name: "encoded https href", svg: `<svg xmlns="http://www.w3.org/2000/svg"><image href="https&#58;//example.test/a.png"/></svg>`},
		{name: "encoded file href", svg: `<svg xmlns="http://www.w3.org/2000/svg"><image href="file&#x3a;///etc/passwd"/></svg>`},
		{name: "absolute xlink href", svg: `<svg xmlns="http://www.w3.org/2000/svg"><use xlink:href="/etc/passwd"/></svg>`},
		{name: "relative href", svg: `<svg xmlns="http://www.w3.org/2000/svg"><image href="other.png"/></svg>`},
		{name: "parent relative href", svg: `<svg xmlns="http://www.w3.org/2000/svg"><image href="../secret.png"/></svg>`},
		{name: "dot relative src", svg: `<svg xmlns="http://www.w3.org/2000/svg"><image src="./asset.svg"/></svg>`},
		{name: "script", svg: `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`},
		{name: "event handler", svg: `<svg xmlns="http://www.w3.org/2000/svg" onclick="alert(1)"></svg>`},
		{name: "entity", svg: `<svg xmlns="http://www.w3.org/2000/svg"><!ENTITY xxe SYSTEM "file:///etc/passwd"></svg>`},
		{name: "style import", svg: `<svg xmlns="http://www.w3.org/2000/svg"><style>@import url(https://example.test/a.css)</style></svg>`},
		{name: "style url", svg: `<svg xmlns="http://www.w3.org/2000/svg"><rect style="fill:url(file:///etc/passwd)"/></svg>`},
		{name: "encoded style url", svg: `<svg xmlns="http://www.w3.org/2000/svg"><rect style="fill:url&#40;file:///etc/passwd)"/></svg>`},
		{name: "encoded style import url", svg: `<svg xmlns="http://www.w3.org/2000/svg"><style>&#64;import url&#40;https://example.test/a.css)</style></svg>`},
		{name: "css escaped style url", svg: `<svg xmlns="http://www.w3.org/2000/svg"><rect style="fill:u\72l(file:///etc/passwd)"/></svg>`},
		{name: "css escaped style import", svg: `<svg xmlns="http://www.w3.org/2000/svg"><style>@im\70ort url(https://example.test/a.css)</style></svg>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "image.svg")
			if err := os.WriteFile(path, []byte(tt.svg), 0o600); err != nil {
				t.Fatalf("write svg fixture: %v", err)
			}

			runner := &fakeRunner{}
			_, err := Load(context.Background(), path, Options{
				LookupExecutable: func(name string) (string, error) {
					if name == "rsvg-convert" {
						return "/test/bin/rsvg-convert", nil
					}
					return "", os.ErrNotExist
				},
				CommandRunner: runner,
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "svg contains external resource reference") {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(runner.calls) != 0 {
				t.Fatalf("runner calls = %d, want 0", len(runner.calls))
			}
		})
	}
}

func TestValidateStaticSVGRejectsUnsafeContent(t *testing.T) {
	tests := []struct {
		name string
		svg  string
	}{
		{name: "doctype system", svg: `<!DOCTYPE svg SYSTEM "http://example.test/a.dtd"><svg xmlns="http://www.w3.org/2000/svg"></svg>`},
		{name: "doctype public", svg: `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://example.test/a.dtd"><svg xmlns="http://www.w3.org/2000/svg"></svg>`},
		{name: "encoded https href", svg: `<svg xmlns="http://www.w3.org/2000/svg"><image href="https&#58;//example.test/a.png"/></svg>`},
		{name: "encoded file href", svg: `<svg xmlns="http://www.w3.org/2000/svg"><image href="file&#x3a;///etc/passwd"/></svg>`},
		{name: "relative href", svg: `<svg xmlns="http://www.w3.org/2000/svg"><image href="other.png"/></svg>`},
		{name: "parent relative href", svg: `<svg xmlns="http://www.w3.org/2000/svg"><image href="../secret.png"/></svg>`},
		{name: "dot relative src", svg: `<svg xmlns="http://www.w3.org/2000/svg"><image src="./asset.svg"/></svg>`},
		{name: "style import", svg: `<svg xmlns="http://www.w3.org/2000/svg"><style>@import url(https://example.test/a.css)</style></svg>`},
		{name: "style url", svg: `<svg xmlns="http://www.w3.org/2000/svg"><rect style="fill:url(file:///etc/passwd)"/></svg>`},
		{name: "encoded style url", svg: `<svg xmlns="http://www.w3.org/2000/svg"><rect style="fill:url&#40;file:///etc/passwd)"/></svg>`},
		{name: "encoded style import url", svg: `<svg xmlns="http://www.w3.org/2000/svg"><style>&#64;import url&#40;https://example.test/a.css)</style></svg>`},
		{name: "css escaped style url", svg: `<svg xmlns="http://www.w3.org/2000/svg"><rect style="fill:u\72l(file:///etc/passwd)"/></svg>`},
		{name: "css escaped style import", svg: `<svg xmlns="http://www.w3.org/2000/svg"><style>@im\70ort url(https://example.test/a.css)</style></svg>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if isSVGStaticSafe([]byte(tt.svg)) {
				t.Fatal("expected unsafe SVG content")
			}
		})
	}
}

func TestValidateStaticSVGAllowsInternalFragmentResource(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><linearGradient id="gradient"></linearGradient><use href="#gradient"/></svg>`
	if !isSVGStaticSafe([]byte(svg)) {
		t.Fatal("expected internal fragment reference to be safe")
	}
}

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

func truncatedPNGPassingDecodeConfig(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{A: 255})

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	pngBytes := out.Bytes()
	if len(pngBytes) < 33 {
		t.Fatalf("encoded png length = %d, want at least IHDR", len(pngBytes))
	}
	truncated := append([]byte(nil), pngBytes[:33]...)
	if _, err := png.DecodeConfig(bytes.NewReader(truncated)); err != nil {
		t.Fatalf("truncated png should pass DecodeConfig: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(truncated)); err == nil {
		t.Fatal("truncated png should fail full Decode")
	}
	return truncated
}

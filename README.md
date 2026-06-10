# iview

`iview` is a small terminal image viewer for Linux terminals with terminal image
protocol support or ANSI color output.

It renders terminal graphics when available and can fall back to ANSI color
blocks. It does not convert images to ASCII.

## Build

```sh
go build -o bin/iview ./cmd/iview
```

## Usage

```sh
iview ./photo.jpg
iview https://example.com/image.png
```

Supported input formats are PNG, JPEG, GIF, WebP, BMP, TIFF, AVIF, SVG, and ICO.
GIF rendering uses the first frame only. SVG rendering uses a static
rasterization path. SVG scripts, event handlers, external resource references,
DTD declarations, and CSS URL imports are rejected.

By default, `iview` auto-selects a renderer. It prefers real terminal image
protocols in this order: Kitty graphics, iTerm2 inline images, and Sixel. If no
real image protocol is detected, it falls back to a terminal-cell approximation
using ANSI color and block characters.

Remote images are downloaded to a temporary file, capped by `--max-bytes`,
validated from magic bytes plus decoder metadata, and removed after rendering.
Non-image downloads are removed and reported as errors.

## Flags

```text
--max-bytes N           maximum downloaded image size, default 52428800
--max-pixels N          maximum decoded image pixels, default 100000000
--max-output-bytes N    maximum normalized image size, default 104857600
--conversion-timeout D  maximum time per external conversion, default 5s
--renderer NAME         renderer: auto, kitty, iterm2, sixel, cells, default auto
--scale-up              scale smaller images up to fill the viewport, default false
--force                 bypass renderer detection; with auto, force Kitty output, default false
--version               print version, default false
```

## Optional tools

Some formats and renderers use external tools when installed:

- `ffmpeg`: AVIF and ICO conversion.
- `rsvg-convert`: static SVG rasterization.
- `magick`: fallback conversion for AVIF and ICO when available.
- `img2sixel`: Sixel output when selected and supported.

Conversion tools are invoked without a shell, with fixed arguments, timeouts,
and private temporary files. They must produce validated PNG output before
anything is rendered. Renderer-side tools such as `img2sixel` are invoked
without a shell, with fixed arguments, timeouts, and private temporary files.
If a required tool is missing, `iview` prints an install hint.

## Safety

`iview` treats local and remote inputs as untrusted. It validates image headers,
limits downloaded bytes, limits decoded pixels, limits normalized output size,
and emits no terminal graphics until normalization succeeds. Remote downloads
are stored in temporary files and removed after rendering or failure.

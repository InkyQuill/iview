# Terminal Image Support Design

## Context

`iview` is a small Go CLI that renders images in terminals. The current app:

- resolves a local path or HTTP(S) URL through `internal/source`;
- validates and decodes bitmap images in `internal/imageio`;
- normalizes decoded images to PNG bytes;
- fits images to the terminal viewport through `internal/terminal`;
- emits Kitty graphics protocol output through `internal/render`.

The current implementation supports PNG, JPEG, GIF, WebP, BMP, and TIFF in Go, but rendering is effectively tied to Kitty-compatible terminals. GIF rendering is first-frame only. The new goal is broader renderability across terminals and formats while keeping malformed or hostile image files from causing destructive side effects.

## Goals

- Support renderable inputs for GIF, BMP, JPEG, PNG, WebP, AVIF, SVG, and ICO.
- Preserve existing TIFF support as a best-effort bitmap path unless a later decision explicitly removes it.
- Keep GIF rendering first-frame only. Animation is out of scope for this design.
- Support terminals beyond Kitty and Ghostty by selecting the best available renderer.
- Provide a terminal-cell fallback for ordinary terminals that cannot render real image protocols.
- Support SVG as a static-safe rasterization path only.
- Use optional external tools such as `ffmpeg`, `rsvg-convert`, `magick`, `img2sixel`, and later `chafa` when installed.
- Produce clear installation hints when an optional tool is required but missing.
- Treat all inputs, including local files, as untrusted.

## Non-Goals

- Animated GIF playback.
- Animated SVG behavior.
- Exact visual fidelity in terminal-cell fallback mode.
- Network or file resource loading from SVG documents.
- A full sandbox or container runtime around external tools.

## Architecture

The app should be organized as three explicit stages:

1. **Source resolution**: keep the current local/HTTP resolver, byte cap, redirect cap, and temporary-file cleanup behavior.
2. **Image normalization**: inspect the file, choose a decoder or converter, and produce bounded PNG bytes plus dimensions.
3. **Terminal rendering**: select a renderer and emit the normalized image through Kitty, iTerm2, Sixel, or terminal cells.

The normalization stage should expose a stable value:

```go
type NormalizedImage struct {
	Width        int
	Height       int
	PNG          []byte
	SourceFormat string
}
```

Renderers should share a small interface:

```go
type Renderer interface {
	Name() string
	Supported(env []string) bool
	Render(w io.Writer, img NormalizedImage, placement terminal.Placement) error
}
```

This keeps source fetching, image conversion, terminal sizing, and protocol emission independently testable.

## Format Handling

Input detection should use magic bytes and limited header parsing. File extensions may be used only as secondary hints when content signatures are ambiguous, such as XML-based SVG.

The guaranteed new support set is GIF, BMP, JPEG, PNG, WebP, AVIF, SVG, and ICO. Existing TIFF decoding should remain available, but TIFF is not part of the new cross-renderer acceptance criteria unless it is documented separately.

Format behavior:

- **PNG, JPEG, GIF, BMP, WebP, TIFF**: decode in Go first where practical. GIF uses the first frame.
- **AVIF**: use `ffmpeg` first when installed. `magick` may be used as a fallback if local ImageMagick supports AVIF.
- **SVG**: use `rsvg-convert` for static-safe rasterization when installed. `magick` should only be used for SVG if the implementation can prevent external resource access reliably.
- **ICO**: use `ffmpeg` or `magick` to select a suitable contained image and normalize it to PNG.
- **Terminal-cell fallback**: use a built-in Unicode/block renderer initially. If `chafa` is installed later, it may become an optional higher-quality adapter.
- **Sixel**: either emit Sixel directly in Go in a later step or use `img2sixel` when installed.

When a required optional tool is missing, the error should name the missing capability and tool. Example: `AVIF support requires ffmpeg. Install ffmpeg or convert the image to PNG/JPEG.`

## Terminal Rendering

Renderer selection defaults to `auto`:

1. Kitty graphics protocol for Kitty, Ghostty, WezTerm, Konsole, and other clearly compatible terminals.
2. iTerm2 inline images for `TERM_PROGRAM=iTerm.app`.
3. Sixel for terminals that advertise Sixel support, or when requested explicitly.
4. Terminal-cell fallback for ordinary terminals.

Flags:

```text
--renderer auto|kitty|iterm2|sixel|cells
--force
--max-bytes N
--max-pixels N
--max-output-bytes N
--conversion-timeout D
--scale-up
```

`--force` means to use the selected renderer even if terminal detection does not recognize support. For backward compatibility, `--renderer=auto --force` should use Kitty output and warn when the terminal is unrecognized.

Terminal-cell fallback should describe itself as an approximation. It renders using terminal colors and block characters rather than a real image protocol.

## Security And Safety

All inputs are untrusted. The app must not perform destructive actions based on malformed image content.

Safety requirements:

- Never execute decoded image content.
- Rasterize SVG as a static document only.
- Do not allow SVG script execution, animation playback, network fetches, or local-file references.
- Never invoke external tools through a shell.
- Use `exec.CommandContext` with fixed argument lists.
- Apply `--max-bytes` to downloads and local files where practical before parsing.
- Apply `--max-pixels` before full decode and again after external conversion.
- Apply a bounded conversion timeout, defaulting to a small value such as 5 seconds per conversion.
- Write external outputs only to private temp directories.
- Validate external outputs as PNG before rendering.
- Enforce `--max-output-bytes` for generated PNGs and other external outputs.
- Normalize image data to PNG and ignore metadata.
- Emit no terminal graphics until validation and normalization succeed.
- Delete temporary files on every failure path.

Temporary cleanup should be centralized. `source.Resolved.Cleanup()` should be the single cleanup path, including validation failures, so the app layer does not need separate `os.Remove` calls.

## External Tool Policy

External tools are optional capabilities. The app should detect them at runtime and choose the safest available adapter. Tool execution must be deterministic:

- no shell;
- no user-controlled flags;
- bounded timeout;
- bounded input and output sizes;
- isolated temporary output directory;
- stderr captured and summarized, not streamed directly into terminal protocol output;
- output validated before use.

Suggested adapter priority:

- AVIF: `ffmpeg`, then `magick`.
- SVG: `rsvg-convert`, then no fallback unless `magick` can be constrained safely.
- ICO: `ffmpeg`, then `magick`.
- Sixel: native renderer when implemented, otherwise `img2sixel`.
- Cells: built-in renderer first; optional `chafa` later.

## Error Handling

Errors should be specific and actionable:

- unsupported input format;
- missing optional tool with an install hint;
- image exceeds byte, pixel, or output limits;
- conversion timed out;
- external tool failed;
- external tool produced invalid or oversized output;
- terminal renderer unsupported unless fallback is available or `--force` is used.

No renderer should emit partial graphics after a failed validation or conversion.

## Testing

Use focused package tests instead of screenshot-based end-to-end tests for the first implementation.

Test groups:

- format detection for PNG, JPEG, GIF, BMP, WebP, AVIF, SVG, and ICO;
- rejection of non-images, malformed images, invalid dimensions, pixel-limit overflow, oversized local files, malformed SVG, and invalid external outputs;
- fake external tool binaries in a temporary `PATH` to verify fixed arguments, timeout handling, missing-tool hints, output validation, and no shell usage;
- renderer auto-selection for Kitty, Ghostty, WezTerm, Konsole, iTerm2, Sixel, and cells fallback;
- renderer output tests for Kitty, iTerm2, Sixel wrappers, and terminal-cell constraints;
- app flow tests for no output before successful validation, cleanup exactly once, force/renderer behavior, and first-frame GIF handling.

Manual visual checks can be used for final sanity, but package-level tests should carry the main regression coverage.

## Implementation Notes

The existing package boundaries are close to the desired structure. The likely changes are:

- rename or expand `imageio.Image` into `NormalizedImage`;
- split format detection from normalization so external adapters can share the same validation rules;
- add a converter adapter layer under `internal/imageio` or a new `internal/convert` package;
- replace `terminal.SupportsKittyGraphics` with renderer selection that returns a concrete renderer;
- add renderer packages or files for Kitty, iTerm2, Sixel, and cells;
- update CLI flags and README.

The implementation should stay scoped to the requested formats, renderer selection, and safety limits. Broad refactors unrelated to those paths are out of scope.

package terminal

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

type Size struct {
	Rows         int
	Columns      int
	WidthPixels  int
	HeightPixels int
}

type Placement struct {
	Columns      int
	Rows         int
	WidthPixels  int
	HeightPixels int
}

func QuerySize(ttyPath string) (Size, error) {
	if ttyPath == "" {
		ttyPath = "/dev/tty"
	}

	f, err := os.OpenFile(ttyPath, unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NDELAY|unix.O_RDONLY, 0)
	if err != nil {
		return Size{}, err
	}
	defer func() { _ = f.Close() }()

	ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return Size{}, err
	}

	size := Size{
		Rows:         int(ws.Row),
		Columns:      int(ws.Col),
		WidthPixels:  int(ws.Xpixel),
		HeightPixels: int(ws.Ypixel),
	}
	if size.Rows <= 0 || size.Columns <= 0 {
		return Size{}, fmt.Errorf("terminal returned invalid cell size %dx%d", size.Columns, size.Rows)
	}
	return size, nil
}

func FitToViewport(imageWidth, imageHeight int, size Size, scaleUp bool) (Placement, error) {
	if imageWidth <= 0 || imageHeight <= 0 {
		return Placement{}, fmt.Errorf("image dimensions must be positive")
	}
	if size.Columns <= 0 || size.Rows <= 0 {
		return Placement{}, fmt.Errorf("terminal dimensions must be positive")
	}

	maxColumns := size.Columns
	maxRows := size.Rows
	if maxRows > 1 {
		maxRows--
	}

	cellWidth := 1.0
	cellHeight := 2.0
	hasPixelDimensions := size.WidthPixels > 0 && size.HeightPixels > 0
	if hasPixelDimensions {
		cellWidth = float64(size.WidthPixels) / float64(size.Columns)
		cellHeight = float64(size.HeightPixels) / float64(size.Rows)
	}

	maxWidthPixels := float64(maxColumns) * cellWidth
	maxHeightPixels := float64(maxRows) * cellHeight
	scale := min(maxWidthPixels/float64(imageWidth), maxHeightPixels/float64(imageHeight))
	if !scaleUp && scale > 1 {
		scale = 1
	}
	if scale <= 0 {
		return Placement{}, fmt.Errorf("unable to fit image in terminal viewport")
	}

	columns := intCeil(float64(imageWidth) * scale / cellWidth)
	rows := intCeil(float64(imageHeight) * scale / cellHeight)
	columns = clamp(columns, 1, maxColumns)
	rows = clamp(rows, 1, maxRows)

	placement := Placement{
		Columns: columns,
		Rows:    rows,
	}
	if hasPixelDimensions {
		placement.WidthPixels = clamp(intCeil(float64(imageWidth)*scale), 1, intCeil(maxWidthPixels))
		placement.HeightPixels = clamp(intCeil(float64(imageHeight)*scale), 1, intCeil(maxHeightPixels))
	}

	return placement, nil
}

func intCeil(value float64) int {
	asInt := int(value)
	if float64(asInt) == value {
		return asInt
	}
	return asInt + 1
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}

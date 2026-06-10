package terminal

import "testing"

func TestFitToViewportShrinksToAvailableCells(t *testing.T) {
	placement, err := FitToViewport(4000, 2000, Size{
		Rows:         50,
		Columns:      100,
		WidthPixels:  1000,
		HeightPixels: 1000,
	}, false)
	if err != nil {
		t.Fatalf("FitToViewport returned error: %v", err)
	}

	if placement.Columns != 100 || placement.Rows != 25 || placement.WidthPixels != 1000 || placement.HeightPixels != 500 {
		t.Fatalf("unexpected placement: %+v", placement)
	}
}

func TestFitToViewportDoesNotScaleUpByDefault(t *testing.T) {
	placement, err := FitToViewport(100, 100, Size{
		Rows:         50,
		Columns:      100,
		WidthPixels:  1000,
		HeightPixels: 1000,
	}, false)
	if err != nil {
		t.Fatalf("FitToViewport returned error: %v", err)
	}

	if placement.Columns != 10 || placement.Rows != 5 || placement.WidthPixels != 100 || placement.HeightPixels != 100 {
		t.Fatalf("unexpected placement: %+v", placement)
	}
}

func TestFitToViewportLeavesPixelDimensionsZeroWhenTerminalDoesNotReportThem(t *testing.T) {
	placement, err := FitToViewport(4000, 2000, Size{
		Rows:    50,
		Columns: 100,
	}, false)
	if err != nil {
		t.Fatalf("FitToViewport returned error: %v", err)
	}

	if placement.Columns != 100 || placement.Rows != 25 || placement.WidthPixels != 0 || placement.HeightPixels != 0 {
		t.Fatalf("unexpected placement: %+v", placement)
	}
}

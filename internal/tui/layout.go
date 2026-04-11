package tui

// Rect defines a rectangular region in the terminal.
type Rect struct {
	X, Y          int
	Width, Height int
}

// Layout holds the computed rectangles for all panels.
type Layout struct {
	Sidebar  Rect
	Details  Rect
	Output   Rect
	Input    Rect // input bar below output
	Status   Rect // status bar at bottom
	Diff     Rect // only used when diff is open

	// Total terminal dimensions.
	Width, Height int

	// Whether the diff panel is open.
	DiffOpen bool
}

const (
	sidebarMinWidth  = 24
	sidebarMaxWidth  = 32
	sidebarFraction  = 4 // 1/4 of terminal width
	statusBarHeight  = 1
	inputHeight      = 3 // rounded border (top + input + bottom)
	minOutputWidth   = 30
	minPanelHeight   = 5
	detailsFraction  = 3 // details gets 1/3 of left column height
)

// ComputeLayout calculates panel positions and sizes for the given terminal dimensions.
func ComputeLayout(width, height int, diffOpen bool) Layout {
	l := Layout{
		Width:    width,
		Height:   height,
		DiffOpen: diffOpen,
	}

	if width < 40 || height < 10 {
		// Degenerate terminal — give everything to output.
		l.Output = Rect{0, 0, width, height}
		return l
	}

	// Status bar at the very bottom.
	l.Status = Rect{
		X: 0, Y: height - statusBarHeight,
		Width: width, Height: statusBarHeight,
	}

	contentHeight := height - statusBarHeight
	if contentHeight < minPanelHeight {
		contentHeight = minPanelHeight
	}

	// Sidebar width: 1/4 of terminal, clamped.
	sidebarW := width / sidebarFraction
	if sidebarW < sidebarMinWidth {
		sidebarW = sidebarMinWidth
	}
	if sidebarW > sidebarMaxWidth {
		sidebarW = sidebarMaxWidth
	}
	if sidebarW > width/2 {
		sidebarW = width / 2
	}

	// Sidebar and details split the left column vertically.
	detailsH := contentHeight / detailsFraction
	if detailsH < minPanelHeight {
		detailsH = minPanelHeight
	}
	sidebarH := contentHeight - detailsH

	l.Sidebar = Rect{X: 0, Y: 0, Width: sidebarW, Height: sidebarH}
	l.Details = Rect{X: 0, Y: sidebarH, Width: sidebarW, Height: detailsH}

	// Right side: output (+ optional diff split).
	rightX := sidebarW
	rightW := width - sidebarW
	rightH := contentHeight

	if diffOpen && rightW >= minOutputWidth*2 {
		// Split right side vertically: output | diff.
		outputW := rightW / 2
		diffW := rightW - outputW

		outputContentH := rightH - inputHeight
		if outputContentH < minPanelHeight {
			outputContentH = minPanelHeight
		}

		l.Output = Rect{X: rightX, Y: 0, Width: outputW, Height: outputContentH}
		l.Input = Rect{X: rightX, Y: outputContentH, Width: outputW, Height: inputHeight}
		l.Diff = Rect{X: rightX + outputW, Y: 0, Width: diffW, Height: rightH}
	} else if diffOpen {
		// Not enough room for split — diff takes the full right side.
		l.Diff = Rect{X: rightX, Y: 0, Width: rightW, Height: rightH}
		l.Output = Rect{X: rightX, Y: 0, Width: 0, Height: 0} // hidden
		l.Input = Rect{X: rightX, Y: 0, Width: 0, Height: 0}  // hidden
	} else {
		// No diff — output takes full right side minus input.
		outputContentH := rightH - inputHeight
		if outputContentH < minPanelHeight {
			outputContentH = minPanelHeight
		}

		l.Output = Rect{X: rightX, Y: 0, Width: rightW, Height: outputContentH}
		l.Input = Rect{X: rightX, Y: outputContentH, Width: rightW, Height: inputHeight}
	}

	return l
}

// ContentWidth returns the usable width inside a bordered panel.
func (r Rect) ContentWidth() int {
	w := r.Width - 2 // left + right border
	if w < 0 {
		return 0
	}
	return w
}

// ContentHeight returns the usable height inside a bordered panel.
func (r Rect) ContentHeight() int {
	h := r.Height - 2 // top + bottom border
	if h < 0 {
		return 0
	}
	return h
}

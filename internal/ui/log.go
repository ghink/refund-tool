package ui

import (
	"fmt"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// logPane is the operator log stream.
type logPane struct {
	grid *widget.TextGrid
}

func newLogPane() *logPane {
	// TextGrid defaults to ScrollNone, where ScrollToBottom panics.
	grid := widget.NewTextGrid()
	grid.Scroll = fyne.ScrollBoth

	return &logPane{grid: grid}
}

func (p *logPane) Info(format string, args ...any) { p.line(nil, format, args...) }
func (p *logPane) OK(format string, args ...any)   { p.line(theme.SuccessColor, format, args...) }
func (p *logPane) Fail(format string, args ...any) { p.line(theme.ErrorColor, format, args...) }
func (p *logPane) Dim(format string, args ...any)  { p.line(theme.PlaceHolderColor, format, args...) }

// line appends one entry and styles every row it wraps into.
func (p *logPane) line(fg func() color.Color, format string, args ...any) {
	text := time.Now().Format("15:04:05  ") + fmt.Sprintf(format, args...)

	first := len(p.grid.Rows)
	p.grid.Append(text)

	if fg != nil {
		style := &widget.CustomTextGridStyle{FGColor: fg()}
		for row := first; row < len(p.grid.Rows); row++ {
			p.grid.SetRowStyle(row, style)
		}
	}

	p.grid.ScrollToBottom()
}

func (p *logPane) Clear() {
	p.grid.SetText("")
}

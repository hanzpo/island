package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// zones is the global zone manager used for mouse click detection.
var zones = zone.New()

// Zone ID helpers for consistent naming between View and Update.
func sidebarZoneID(i int) string    { return fmt.Sprintf("ws-%d", i) }
func sessionTabZoneID(i int) string { return fmt.Sprintf("tab-%d", i) }
func panelZoneID(p PanelID) string  { return fmt.Sprintf("panel-%d", p) }

// PanelID identifies a focusable panel in the layout.
type PanelID int

const (
	PanelWorkspaces PanelID = iota
	PanelDetails
	PanelOutput
	PanelDiff
	PanelCount // sentinel — total number of panels (excluding diff)
)

// --- Color Palette ---

var (
	// Base colors
	colorBorder       = lipgloss.Color("240") // dim gray for inactive borders
	colorBorderActive = lipgloss.Color("63")  // blue-purple for active panel
	colorTitle        = lipgloss.Color("63")  // panel titles
	colorTitleActive  = lipgloss.Color("99")  // active panel title
	colorSubtle       = lipgloss.Color("241") // faint text
	colorText         = lipgloss.Color("252") // normal text
	colorBright       = lipgloss.Color("255") // bright/bold text

	// Status colors
	colorRunning   = lipgloss.Color("212") // pink/magenta for active
	colorCompleted = lipgloss.Color("78")  // green
	colorError     = lipgloss.Color("196") // red
	colorWarning   = lipgloss.Color("214") // orange
	colorWaiting   = lipgloss.Color("214") // orange
	colorInReview  = lipgloss.Color("99")  // purple
	colorCancelled = lipgloss.Color("241") // gray

	// Diff colors
	colorDiffAdded   = lipgloss.Color("78")  // green
	colorDiffRemoved = lipgloss.Color("196") // red
	colorDiffHunk    = lipgloss.Color("44")  // cyan
	colorDiffHeader  = lipgloss.Color("63")  // blue

	// Misc
	colorInputBorder = lipgloss.Color("240")
	colorPlaceholder = lipgloss.Color("241")
	colorSelection   = lipgloss.Color("63")
	colorBadge       = lipgloss.Color("99")
)

// --- Panel Rendering ---

// buildPanel constructs a bordered panel with a title in the top border.
// Width and height are the TOTAL dimensions including borders.
// Content is rendered inside the border.
func buildPanel(title string, _ bool, width, height int, content string) string {
	if width < 4 || height < 3 {
		return ""
	}

	bdr := lipgloss.NewStyle().Foreground(colorBorder)
	ttl := lipgloss.NewStyle().Bold(true).Foreground(colorTitle)

	cw := width - 2 // content width (inside left+right border chars)
	lb := bdr.Render("\u2502")

	// Top border: ╭─ Title ────────╮
	titleStr := ttl.Render(" " + title + " ")
	titleVW := lipgloss.Width(titleStr)
	topFill := cw - 1 - titleVW // 1 for the ─ after ╭
	if topFill < 0 {
		topFill = 0
	}
	topBorder := bdr.Render("\u256d\u2500") + titleStr + bdr.Render(strings.Repeat("\u2500", topFill)+"\u256e")

	// Bottom border: ╰──────────────╯
	botBorder := bdr.Render("\u2570" + strings.Repeat("\u2500", cw) + "\u256f")

	// Content lines — pad to fill exactly.
	contentLines := strings.Split(content, "\n")
	innerHeight := height - 2

	var b strings.Builder
	b.WriteString(topBorder)
	for i := 0; i < innerHeight; i++ {
		b.WriteByte('\n')
		line := ""
		if i < len(contentLines) {
			line = contentLines[i]
		}
		pad := cw - lipgloss.Width(line)
		if pad < 0 {
			pad = 0
		}
		b.WriteString(lb)
		b.WriteString(line)
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(lb)
	}
	b.WriteByte('\n')
	b.WriteString(botBorder)

	return b.String()
}

// --- Status Styles ---

var (
	styleRunning = lipgloss.NewStyle().
			Foreground(colorRunning).
			Bold(true)

	styleCompleted = lipgloss.NewStyle().
			Foreground(colorCompleted)

	styleErrored = lipgloss.NewStyle().
			Foreground(colorError)

	styleWarning = lipgloss.NewStyle().
			Foreground(colorWarning)

	styleWaiting = lipgloss.NewStyle().
			Foreground(colorWaiting)

	styleInReview = lipgloss.NewStyle().
			Foreground(colorInReview)

	styleCancelled = lipgloss.NewStyle().
			Faint(true).
			Foreground(colorCancelled)

	styleArchived = lipgloss.NewStyle().
			Faint(true).
			Foreground(colorSubtle)
)

// --- Text Styles ---

var (
	styleSubtle = lipgloss.NewStyle().
			Foreground(colorSubtle)

	styleText = lipgloss.NewStyle().
			Foreground(colorText)

	styleBright = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBright)

	styleBold = lipgloss.NewStyle().
			Bold(true)

	styleLabel = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Bold(true)

	styleValue = lipgloss.NewStyle().
			Foreground(colorText)

	styleBadge = lipgloss.NewStyle().
			Foreground(colorBadge).
			Bold(true)
)

// --- Diff Styles ---

var (
	styleDiffAdded = lipgloss.NewStyle().
			Foreground(colorDiffAdded)

	styleDiffRemoved = lipgloss.NewStyle().
				Foreground(colorDiffRemoved)

	styleDiffHunk = lipgloss.NewStyle().
			Foreground(colorDiffHunk)

	styleDiffHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorDiffHeader)

	styleDiffContext = lipgloss.NewStyle().
			Faint(true)
)

// --- Session Tab Styles ---

var (
	styleTab = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Padding(0, 1)

	styleTabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBorderActive).
			Padding(0, 1)
)

// --- Tool / Output Styles ---

var (
	styleToolLine = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Faint(true)

	styleUserMsg = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	styleThinking = lipgloss.NewStyle().
			Faint(true).
			Italic(true)

	styleStderr = lipgloss.NewStyle().
			Faint(true).
			Foreground(colorWarning)
)

// --- Dialog Styles ---

var (
	styleDialog = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorderActive).
			Padding(1, 2)

	styleDialogTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTitleActive).
			Padding(0, 0, 1, 0)

	styleSelected = lipgloss.NewStyle().
			Foreground(colorSelection).
			Bold(true)

	styleNormal = lipgloss.NewStyle().
			Foreground(colorSubtle)
)

// --- Input Style ---

var styleInput = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorInputBorder).
	Padding(0, 1)

// --- Statusbar Style ---

var (
	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorSubtle)

	styleStatusKey = lipgloss.NewStyle().
			Foreground(colorBorderActive).
			Bold(true)

	styleStatusSep = lipgloss.NewStyle().
			Foreground(colorBorder)
)

// Spinner animation frames for active indicators.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

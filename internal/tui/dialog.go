package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hanz/island/internal/config"
)

// DialogMode distinguishes between creating a new workspace and adding an
// agent to an existing workspace.
type DialogMode int

const (
	ModeNewWorkspace DialogMode = iota
	ModeAddAgent
)

// DialogModel is the overlay dialog for creating a new workspace or adding an agent.
type DialogModel struct {
	open bool
	mode DialogMode

	// Agent selection
	agents       []string
	agentIdx     int
	defaultAgent string

	// Result
	confirmed bool
	agentName string

	keys DialogKeys
}

func newDialogModel(cfg *config.Config) DialogModel {
	agents := make([]string, 0, len(cfg.Agents))
	for name := range cfg.Agents {
		agents = append(agents, name)
	}
	sort.Strings(agents)

	defaultAgent := cfg.General.DefaultAgent
	defaultIdx := 0
	for i, name := range agents {
		if name == defaultAgent {
			defaultIdx = i
			break
		}
	}

	return DialogModel{
		agents:       agents,
		agentIdx:     defaultIdx,
		defaultAgent: defaultAgent,
		keys:         DefaultDialogKeys(),
	}
}

// Open resets and shows the dialog in new-workspace mode.
func (d *DialogModel) Open() {
	d.open = true
	d.mode = ModeNewWorkspace
	d.confirmed = false
	d.agentName = ""

	d.agentIdx = 0
	for i, name := range d.agents {
		if name == d.defaultAgent {
			d.agentIdx = i
			break
		}
	}
}

// OpenAddAgent shows the dialog in add-agent mode.
func (d *DialogModel) OpenAddAgent() {
	d.open = true
	d.mode = ModeAddAgent
	d.confirmed = false
	d.agentName = ""

	d.agentIdx = 0
	for i, name := range d.agents {
		if name == d.defaultAgent {
			d.agentIdx = i
			break
		}
	}
}

// IsOpen returns true if the dialog is visible.
func (d *DialogModel) IsOpen() bool {
	return d.open
}

// Update handles input for the dialog.
func (d *DialogModel) Update(msg tea.Msg) tea.Cmd {
	if !d.open {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, d.keys.Cancel) {
			d.open = false
			return nil
		}

		if key.Matches(msg, d.keys.Select) {
			return d.tryConfirm()
		}

		switch {
		case key.Matches(msg, d.keys.Left):
			if d.agentIdx > 0 {
				d.agentIdx--
			}
		case key.Matches(msg, d.keys.Right):
			if d.agentIdx < len(d.agents)-1 {
				d.agentIdx++
			}
		}
	}

	return nil
}

func (d *DialogModel) tryConfirm() tea.Cmd {
	if len(d.agents) > 0 {
		d.agentName = d.agents[d.agentIdx]
	}
	d.confirmed = true
	d.open = false
	return nil
}

// View renders the dialog overlay.
func (d *DialogModel) View(width, height int) string {
	if !d.open {
		return ""
	}

	const dialogWidth = 52
	var b strings.Builder

	if d.mode == ModeAddAgent {
		b.WriteString(styleDialogTitle.Render("Add Agent"))
	} else {
		b.WriteString(styleDialogTitle.Render("New Workspace"))
	}
	b.WriteByte('\n')

	// Agent field
	agentLabel := styleSelected.Render("Agent:")

	agentName := ""
	if len(d.agents) > 0 {
		agentName = d.agents[d.agentIdx]
	}
	agentDisplay := agentName
	if agentName == d.defaultAgent {
		agentDisplay += " (default)"
	}
	agentDisplay = styleSelected.Render(agentDisplay)
	hint := styleSubtle.Render("  [←/→]")
	b.WriteString(agentLabel + "       " + agentDisplay + hint)

	b.WriteString("\n\n")

	if d.mode == ModeAddAgent {
		b.WriteString(styleSubtle.Render("Enter: add • Esc: cancel"))
	} else {
		b.WriteString(styleSubtle.Render("Enter: create • Esc: cancel"))
	}

	content := b.String()
	styledDialog := styleDialog.Width(dialogWidth).Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, styledDialog)
}

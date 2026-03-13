package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hanz/island/internal/config"
)

// DialogField identifies which field has focus in the new-workspace dialog.
type DialogField int

const (
	FieldTask     DialogField = iota // primary: task description
	FieldAgent                       // agent selection
	FieldTemplate                    // template selection
)

// DialogMode distinguishes between creating a new workspace and adding an
// agent to an existing workspace.
type DialogMode int

const (
	ModeNewWorkspace DialogMode = iota
	ModeAddAgent                // agent-only picker for existing workspace
)

// DialogModel is the overlay dialog for creating a new workspace or adding
// an agent to an existing one.
type DialogModel struct {
	open        bool
	mode        DialogMode
	activeField DialogField

	// Task input (primary field, only used in ModeNewWorkspace).
	taskInput textinput.Model

	// Agent selection.
	agents       []string // sorted agent names
	agentIdx     int
	defaultAgent string // from config

	// Template selection (only used in ModeNewWorkspace).
	templates   []string // "" + sorted template names
	templateIdx int

	// Result (set when confirmed).
	confirmed    bool
	agentName    string
	templateName string // "" if no template
	taskText     string

	keys DialogKeyMap
}

func newDialogModel(cfg *config.Config) DialogModel {
	// Collect and sort agent names.
	agents := make([]string, 0, len(cfg.Agents))
	for name := range cfg.Agents {
		agents = append(agents, name)
	}
	sort.Strings(agents)

	// Find default agent index.
	defaultAgent := cfg.General.DefaultAgent
	defaultIdx := 0
	for i, name := range agents {
		if name == defaultAgent {
			defaultIdx = i
			break
		}
	}

	// Collect and sort template names; first entry is "" (none).
	templates := []string{""}
	tkeys := make([]string, 0, len(cfg.Templates))
	for name := range cfg.Templates {
		tkeys = append(tkeys, name)
	}
	sort.Strings(tkeys)
	templates = append(templates, tkeys...)

	ti := textinput.New()
	ti.Placeholder = "Describe the task..."
	ti.CharLimit = 500
	ti.Width = 50

	return DialogModel{
		open:         false,
		activeField:  FieldTask,
		taskInput:    ti,
		agents:       agents,
		agentIdx:     defaultIdx,
		defaultAgent: defaultAgent,
		templates:    templates,
		templateIdx:  0,
		keys:         defaultDialogKeys(),
	}
}

// Open resets and shows the dialog in new-workspace mode.
func (d *DialogModel) Open() {
	d.open = true
	d.mode = ModeNewWorkspace
	d.activeField = FieldAgent
	d.confirmed = false
	d.agentName = ""
	d.templateName = ""
	d.taskText = ""
	d.taskInput.Blur()

	// Reset agent to default.
	d.agentIdx = 0
	for i, name := range d.agents {
		if name == d.defaultAgent {
			d.agentIdx = i
			break
		}
	}

	// Reset template to "none".
	d.templateIdx = 0
}

// OpenAddAgent shows the dialog in add-agent mode (agent picker only).
func (d *DialogModel) OpenAddAgent() {
	d.open = true
	d.mode = ModeAddAgent
	d.activeField = FieldAgent
	d.confirmed = false
	d.agentName = ""
	d.templateName = ""
	d.taskText = ""
	d.taskInput.Blur()

	// Reset agent to default.
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

// Update handles input for the dialog. Returns a tea.Cmd (for textinput blink).
func (d *DialogModel) Update(msg tea.Msg) tea.Cmd {
	if !d.open {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Esc always cancels.
		if key.Matches(msg, d.keys.Cancel) {
			d.open = false
			d.taskInput.Blur()
			return nil
		}

		// Tab cycles fields.
		if key.Matches(msg, d.keys.Next) {
			return d.cycleField()
		}

		// Enter creates workspace (only if task is non-empty).
		if key.Matches(msg, d.keys.Select) {
			return d.tryConfirm()
		}

		// Field-specific handling.
		switch d.activeField {
		case FieldTask:
			var cmd tea.Cmd
			d.taskInput, cmd = d.taskInput.Update(msg)
			return cmd
		case FieldAgent:
			return d.updateAgentField(msg)
		case FieldTemplate:
			return d.updateTemplateField(msg)
		}
	}

	// Propagate non-key messages to textinput (for cursor blink).
	if d.activeField == FieldTask {
		var cmd tea.Cmd
		d.taskInput, cmd = d.taskInput.Update(msg)
		return cmd
	}

	return nil
}

func (d *DialogModel) tryConfirm() tea.Cmd {
	if len(d.agents) > 0 {
		d.agentName = d.agents[d.agentIdx]
	}

	if d.mode == ModeNewWorkspace {
		d.templateName = d.templates[d.templateIdx]
	}

	d.confirmed = true
	d.open = false
	d.taskInput.Blur()
	return nil
}

func (d *DialogModel) cycleField() tea.Cmd {
	if d.mode == ModeAddAgent {
		// Only the agent field is available.
		return nil
	}
	// ModeNewWorkspace: cycle between agent and template only.
	switch d.activeField {
	case FieldAgent:
		d.activeField = FieldTemplate
	case FieldTemplate:
		d.activeField = FieldAgent
	}
	return nil
}

func (d *DialogModel) updateAgentField(msg tea.KeyMsg) tea.Cmd {
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
	return nil
}

func (d *DialogModel) updateTemplateField(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, d.keys.Left):
		if d.templateIdx > 0 {
			d.templateIdx--
		}
	case key.Matches(msg, d.keys.Right):
		if d.templateIdx < len(d.templates)-1 {
			d.templateIdx++
		}
	}
	return nil
}

// View renders the dialog overlay centered in the given dimensions.
func (d *DialogModel) View(width, height int) string {
	if !d.open {
		return ""
	}

	const dialogWidth = 56
	var b strings.Builder

	if d.mode == ModeAddAgent {
		b.WriteString(dialogTitleStyle.Render("Add Agent to Workspace"))
	} else {
		b.WriteString(dialogTitleStyle.Render("New Workspace"))
	}
	b.WriteByte('\n')

	// Agent field.
	agentLabel := "Agent:"
	if d.activeField == FieldAgent {
		agentLabel = selectedItemStyle.Render("Agent:")
	} else {
		agentLabel = normalItemStyle.Render("Agent:")
	}

	agentName := ""
	if len(d.agents) > 0 {
		agentName = d.agents[d.agentIdx]
	}
	agentDisplay := agentName
	if agentName == d.defaultAgent {
		agentDisplay += " (default)"
	}
	if d.activeField == FieldAgent {
		agentDisplay = selectedItemStyle.Render(agentDisplay)
	}
	tabHint := footerStyle.Render("  [\u2190/\u2192: change]")
	b.WriteString(agentLabel + "    " + agentDisplay + tabHint)
	b.WriteByte('\n')

	// Template field (only in new-workspace mode).
	if d.mode == ModeNewWorkspace {
		templateLabel := "Template:"
		if d.activeField == FieldTemplate {
			templateLabel = selectedItemStyle.Render("Template:")
		} else {
			templateLabel = normalItemStyle.Render("Template:")
		}

		templateName := d.templates[d.templateIdx]
		templateDisplay := templateName
		if templateDisplay == "" {
			templateDisplay = "none"
		}
		if d.activeField == FieldTemplate {
			templateDisplay = selectedItemStyle.Render(templateDisplay)
		}
		tTabHint := ""
		if d.activeField != FieldTemplate {
			tTabHint = footerStyle.Render("  [Tab: change]")
		} else {
			tTabHint = footerStyle.Render("  [\u2190/\u2192: change]")
		}
		b.WriteString(templateLabel + " " + templateDisplay + tTabHint)
	}
	b.WriteString("\n\n")

	// Footer hints.
	if d.mode == ModeAddAgent {
		b.WriteString(footerStyle.Render("Enter: add \u2022 Esc: cancel"))
	} else {
		b.WriteString(footerStyle.Render("Enter: create \u2022 Esc: cancel"))
	}

	content := b.String()

	styledDialog := dialogStyle.
		Width(dialogWidth).
		Render(content)

	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		styledDialog,
	)
}

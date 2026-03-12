package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hanz/island/internal/config"
)

// DialogStep represents which step of the new-workspace dialog is active.
type DialogStep int

const (
	StepSelectBackend DialogStep = iota
	StepSelectTemplate
	StepEnterTask
	StepClosed
)

// DialogModel is the overlay dialog for creating a new workspace.
type DialogModel struct {
	step DialogStep

	// Backend selection
	backends   []string
	backendIdx int

	// Template selection
	templates   []string
	templateIdx int

	// Task input
	taskInput textinput.Model

	// Result (set when confirmed)
	confirmed    bool
	backendName  string
	templateName string // "" if no template selected
	taskText     string

	keys DialogKeyMap
}

func newDialogModel(cfg *config.Config) DialogModel {
	// Collect and sort backend names.
	backends := make([]string, 0, len(cfg.Backends))
	for name := range cfg.Backends {
		backends = append(backends, name)
	}
	sort.Strings(backends)

	// Collect and sort template names; first entry is "" (no template).
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
		step:      StepClosed,
		backends:  backends,
		templates: templates,
		taskInput: ti,
		keys:      defaultDialogKeys(),
	}
}

// Open resets the dialog to the first step.
func (d *DialogModel) Open() {
	d.step = StepSelectBackend
	d.backendIdx = 0
	d.templateIdx = 0
	d.confirmed = false
	d.backendName = ""
	d.templateName = ""
	d.taskText = ""
	d.taskInput.SetValue("")
	d.taskInput.Blur()
}

// IsOpen returns true if the dialog is visible.
func (d *DialogModel) IsOpen() bool {
	return d.step != StepClosed
}

// Update handles input for the dialog. Returns a tea.Cmd (for the textinput).
func (d *DialogModel) Update(msg tea.Msg) tea.Cmd {
	if !d.IsOpen() {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch d.step {
		case StepSelectBackend:
			return d.updateBackendStep(msg)
		case StepSelectTemplate:
			return d.updateTemplateStep(msg)
		case StepEnterTask:
			return d.updateTaskStep(msg)
		}
	}

	// Propagate to textinput if in task step.
	if d.step == StepEnterTask {
		var cmd tea.Cmd
		d.taskInput, cmd = d.taskInput.Update(msg)
		return cmd
	}

	return nil
}

func (d *DialogModel) updateBackendStep(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, d.keys.Cancel):
		d.step = StepClosed
	case key.Matches(msg, d.keys.Up):
		if d.backendIdx > 0 {
			d.backendIdx--
		}
	case key.Matches(msg, d.keys.Down):
		if d.backendIdx < len(d.backends)-1 {
			d.backendIdx++
		}
	case key.Matches(msg, d.keys.Select):
		if len(d.backends) > 0 {
			d.backendName = d.backends[d.backendIdx]
			d.step = StepSelectTemplate
			d.templateIdx = 0
		}
	}
	return nil
}

func (d *DialogModel) updateTemplateStep(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, d.keys.Cancel):
		d.step = StepClosed
	case key.Matches(msg, d.keys.Up):
		if d.templateIdx > 0 {
			d.templateIdx--
		}
	case key.Matches(msg, d.keys.Down):
		if d.templateIdx < len(d.templates)-1 {
			d.templateIdx++
		}
	case key.Matches(msg, d.keys.Select):
		d.templateName = d.templates[d.templateIdx]
		d.step = StepEnterTask
		d.taskInput.Focus()
		return textinput.Blink
	}
	return nil
}

func (d *DialogModel) updateTaskStep(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, d.keys.Cancel):
		d.step = StepClosed
		d.taskInput.Blur()
		return nil
	case key.Matches(msg, d.keys.Select):
		value := strings.TrimSpace(d.taskInput.Value())
		if value != "" {
			d.taskText = value
			d.confirmed = true
			d.step = StepClosed
			d.taskInput.Blur()
		}
		return nil
	default:
		var cmd tea.Cmd
		d.taskInput, cmd = d.taskInput.Update(msg)
		return cmd
	}
}

// View renders the dialog overlay centered in the given dimensions.
func (d *DialogModel) View(width, height int) string {
	if !d.IsOpen() {
		return ""
	}

	const dialogWidth = 60
	var b strings.Builder

	switch d.step {
	case StepSelectBackend:
		b.WriteString(dialogTitleStyle.Render("Select Backend"))
		b.WriteByte('\n')
		for i, name := range d.backends {
			cursor := "  "
			style := normalItemStyle
			if i == d.backendIdx {
				cursor = "> "
				style = selectedItemStyle
			}
			b.WriteString(style.Render(cursor + name))
			if i < len(d.backends)-1 {
				b.WriteByte('\n')
			}
		}

	case StepSelectTemplate:
		b.WriteString(dialogTitleStyle.Render("Select Template (optional)"))
		b.WriteByte('\n')
		for i, name := range d.templates {
			cursor := "  "
			style := normalItemStyle
			if i == d.templateIdx {
				cursor = "> "
				style = selectedItemStyle
			}
			label := name
			if label == "" {
				label = "None (raw prompt)"
			}
			b.WriteString(style.Render(cursor + label))
			if i < len(d.templates)-1 {
				b.WriteByte('\n')
			}
		}

	case StepEnterTask:
		b.WriteString(dialogTitleStyle.Render("Task Description"))
		b.WriteByte('\n')
		b.WriteString(d.taskInput.View())
	}

	content := b.String()

	styledDialog := dialogStyle.
		Width(dialogWidth).
		Render(content)

	// Center the dialog in the terminal.
	return lipgloss.Place(
		width, height,
		lipgloss.Center, lipgloss.Center,
		styledDialog,
	)
}

// stepIndicator returns a human-readable indicator of dialog progress.
func (d *DialogModel) stepIndicator() string {
	switch d.step {
	case StepSelectBackend:
		return "Step 1/3"
	case StepSelectTemplate:
		return "Step 2/3"
	case StepEnterTask:
		return "Step 3/3"
	default:
		return ""
	}
}

// String implements fmt.Stringer for DialogStep (used in debugging).
func (s DialogStep) String() string {
	switch s {
	case StepSelectBackend:
		return "SelectBackend"
	case StepSelectTemplate:
		return "SelectTemplate"
	case StepEnterTask:
		return "EnterTask"
	case StepClosed:
		return "Closed"
	default:
		return fmt.Sprintf("DialogStep(%d)", int(s))
	}
}

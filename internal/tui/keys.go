package tui

import "github.com/charmbracelet/bubbles/key"

// Keys holds all keybindings for the application.
type Keys struct {
	// Navigation
	Up   key.Binding
	Down key.Binding
	Tab  key.Binding

	// Panel jumping
	Panel1 key.Binding
	Panel2 key.Binding
	Panel3 key.Binding

	// Workspace actions
	New     key.Binding
	Agent   key.Binding
	Diff    key.Binding
	Merge   key.Binding
	PR      key.Binding
	Discard key.Binding
	Cancel  key.Binding

	// Output navigation
	PageUp   key.Binding
	PageDown key.Binding
	Top      key.Binding
	Bottom   key.Binding
	NextTab  key.Binding
	PrevTab  key.Binding

	// Global
	Help    key.Binding
	Enter   key.Binding
	Escape  key.Binding
	Quit    key.Binding
	ForceQuit key.Binding
}

// DefaultKeys returns the default vim-style keybindings.
func DefaultKeys() Keys {
	return Keys{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("j/k", "navigate"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j/k", "navigate"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next panel"),
		),
		Panel1: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "workspaces"),
		),
		Panel2: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "details"),
		),
		Panel3: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "output"),
		),
		New: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new workspace"),
		),
		Agent: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "add agent"),
		),
		Diff: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "toggle diff"),
		),
		Merge: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "merge"),
		),
		PR: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "create/merge PR"),
		),
		Discard: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "discard"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "cancel agent"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdown", "page down"),
		),
		Top: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "bottom"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "next session"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "prev session"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select/submit"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back/close"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "quit"),
		),
		ForceQuit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "force quit"),
		),
	}
}

// DialogKeys defines key bindings for the dialog overlay.
type DialogKeys struct {
	Next   key.Binding
	Left   key.Binding
	Right  key.Binding
	Select key.Binding
	Cancel key.Binding
}

// DefaultDialogKeys returns dialog keybindings.
func DefaultDialogKeys() DialogKeys {
	return DialogKeys{
		Next: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next field"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("left", "prev"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("right", "next"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	}
}

// HelpBinding is a keybinding with a description for the help overlay.
type HelpBinding struct {
	Key  string
	Desc string
}

// GlobalHelp returns help bindings visible regardless of context.
func GlobalHelp() []HelpBinding {
	return []HelpBinding{
		{"j/k", "navigate workspaces"},
		{"tab", "cycle panels"},
		{"1/2/3", "jump to panel"},
		{"?", "toggle help"},
		{"q", "quit"},
		{"ctrl+c", "force quit"},
	}
}

// ActionHelp returns help bindings for workspace actions.
func ActionHelp() []HelpBinding {
	return []HelpBinding{
		{"n", "new workspace"},
		{"a", "add agent"},
		{"d", "toggle diff"},
		{"m", "merge"},
		{"p", "create/merge PR"},
		{"x", "discard"},
		{"c", "cancel agent"},
		{"enter", "focus input"},
		{"esc", "back/blur"},
	}
}

// OutputHelp returns help bindings for the output panel.
func OutputHelp() []HelpBinding {
	return []HelpBinding{
		{"pgup/pgdn", "scroll"},
		{"g/G", "top/bottom"},
		{"[/]", "prev/next session"},
	}
}

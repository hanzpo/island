package tui

import "github.com/charmbracelet/bubbles/key"

// DashboardKeyMap defines key bindings for the dashboard screen.
type DashboardKeyMap struct {
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding
	Enter key.Binding
	New   key.Binding
	Diff  key.Binding
	Merge key.Binding
	Discard key.Binding
	Quit  key.Binding
}

// WorkspaceKeyMap defines key bindings for the workspace focus view.
type WorkspaceKeyMap struct {
	Back     key.Binding
	Cancel   key.Binding
	Diff     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Top      key.Binding
	Bottom   key.Binding
}

// DiffKeyMap defines key bindings for the diff review screen.
type DiffKeyMap struct {
	Back     key.Binding
	Merge    key.Binding
	Discard  key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Top      key.Binding
	Bottom   key.Binding
}

// DialogKeyMap defines key bindings for the dialog overlay.
type DialogKeyMap struct {
	Up     key.Binding
	Down   key.Binding
	Select key.Binding
	Cancel key.Binding
}

func defaultDashboardKeys() DashboardKeyMap {
	return DashboardKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("k/up", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j/down", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("h/left", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("l/right", "right"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "focus workspace"),
		),
		New: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new workspace"),
		),
		Diff: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "view diff"),
		),
		Merge: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "merge"),
		),
		Discard: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "discard"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

func defaultWorkspaceKeys() WorkspaceKeyMap {
	return WorkspaceKeyMap{
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "cancel agent"),
		),
		Diff: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "view diff"),
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
	}
}

func defaultDiffKeys() DiffKeyMap {
	return DiffKeyMap{
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Merge: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "merge"),
		),
		Discard: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "discard"),
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
	}
}

func defaultDialogKeys() DialogKeyMap {
	return DialogKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("k/up", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j/down", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	}
}

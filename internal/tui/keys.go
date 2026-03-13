package tui

import "github.com/charmbracelet/bubbles/key"

// MainKeyMap defines key bindings for the main screen (sidebar + workspace).
type MainKeyMap struct {
	Up          key.Binding
	Down        key.Binding
	Enter       key.Binding
	New         key.Binding
	AddAgent    key.Binding
	Diff        key.Binding
	CreatePR    key.Binding
	Merge       key.Binding
	Discard     key.Binding
	NextSession key.Binding
	PrevSession key.Binding
	Quit        key.Binding
}

// WorkspaceKeyMap defines key bindings for the workspace view (right panel).
type WorkspaceKeyMap struct {
	Cancel   key.Binding
	Diff     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Top      key.Binding
	Bottom   key.Binding
}

// DiffKeyMap defines key bindings for the full-screen diff review.
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
	Next   key.Binding
	Left   key.Binding
	Right  key.Binding
	Select key.Binding
	Cancel key.Binding
}

func defaultMainKeys() MainKeyMap {
	return MainKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("up", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("down", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "send/focus"),
		),
		New: key.NewBinding(
			key.WithKeys("ctrl+n"),
			key.WithHelp("ctrl+n", "new workspace"),
		),
		AddAgent: key.NewBinding(
			key.WithKeys("ctrl+a"),
			key.WithHelp("ctrl+a", "add agent"),
		),
		Diff: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "view diff"),
		),
		CreatePR: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("ctrl+p", "create PR"),
		),
		Merge: key.NewBinding(
			key.WithKeys("ctrl+m"),
			key.WithHelp("ctrl+m", "merge"),
		),
		Discard: key.NewBinding(
			key.WithKeys("ctrl+x"),
			key.WithHelp("ctrl+x", "discard"),
		),
		NextSession: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next agent"),
		),
		PrevSession: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev agent"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
	}
}

func defaultWorkspaceKeys() WorkspaceKeyMap {
	return WorkspaceKeyMap{
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
		Next: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next field"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("left", "prev option"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("right", "next option"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "create"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	}
}

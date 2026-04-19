// Package theme holds the Borg-flavored color palette used by the CLI —
// greens for the collective, amber/red/cyan for alerts and data streams.
package theme

import (
	"os"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

var (
	// Primary greens (the collective).
	BorgGreen   = lipgloss.AdaptiveColor{Light: "#0B6623", Dark: "#00FF41"} // bright HUD green
	HiveGreen   = lipgloss.AdaptiveColor{Light: "#064A1C", Dark: "#5EFF8B"} // accent bright
	DeepGreen   = lipgloss.AdaptiveColor{Light: "#10412A", Dark: "#0B6623"} // deep borders
	MutedGreen  = lipgloss.AdaptiveColor{Light: "#4F6F52", Dark: "#4F6F52"} // dim labels
	CorpseGreen = lipgloss.AdaptiveColor{Light: "#8A9A8E", Dark: "#6B7A6E"} // disabled

	// Accents.
	AlertAmber = lipgloss.AdaptiveColor{Light: "#B37400", Dark: "#FFB000"} // in-flight
	AlertRed   = lipgloss.AdaptiveColor{Light: "#C02A2A", Dark: "#FF3B3B"} // failure
	DataCyan   = lipgloss.AdaptiveColor{Light: "#007A99", Dark: "#00E5FF"} // details/data
	Foreground = lipgloss.AdaptiveColor{Light: "#1A2B1F", Dark: "#C8E6C9"} // body text
)

// Styles for plain-output rendering (tables, headers, status).
var (
	Header   = lipgloss.NewStyle().Foreground(HiveGreen).Bold(true)
	Name     = lipgloss.NewStyle().Foreground(BorgGreen).Bold(true)
	Accent   = lipgloss.NewStyle().Foreground(DataCyan)
	Dim      = lipgloss.NewStyle().Foreground(MutedGreen)
	Body     = lipgloss.NewStyle().Foreground(Foreground)
	Success  = lipgloss.NewStyle().Foreground(BorgGreen).Bold(true)
	Running  = lipgloss.NewStyle().Foreground(AlertAmber)
	Failure  = lipgloss.NewStyle().Foreground(AlertRed).Bold(true)
	Pending  = lipgloss.NewStyle().Foreground(CorpseGreen)
	BoolYes  = lipgloss.NewStyle().Foreground(BorgGreen)
	BoolNo   = lipgloss.NewStyle().Foreground(CorpseGreen)
)

// Enabled reports whether to emit color escapes. Honors NO_COLOR and
// requires stdout be a terminal.
func Enabled() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// Phase colorizes a Task/Agent phase string.
func Phase(p string) string {
	if !Enabled() {
		return p
	}
	switch p {
	case "Succeeded":
		return Success.Render(p)
	case "Failed", "TimedOut":
		return Failure.Render(p)
	case "Running", "Dispatching":
		return Running.Render(p)
	case "Pending", "":
		return Pending.Render(p)
	}
	return Accent.Render(p)
}

// Bool colorizes a true/false-style string.
func Bool(v string) string {
	if !Enabled() {
		return v
	}
	switch v {
	case "true":
		return BoolYes.Render(v)
	case "false":
		return BoolNo.Render(v)
	}
	return v
}

// Style wraps s with style if color enabled.
func Style(st lipgloss.Style, s string) string {
	if !Enabled() {
		return s
	}
	return st.Render(s)
}

// Huh returns the Borg-flavored theme for charmbracelet/huh forms.
func Huh() *huh.Theme {
	t := huh.ThemeBase()

	button := lipgloss.NewStyle().Padding(0, 2).MarginRight(1)

	t.Focused.Base = t.Focused.Base.BorderForeground(DeepGreen)
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = t.Focused.Title.Foreground(HiveGreen).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(HiveGreen).Bold(true).MarginBottom(1)
	t.Focused.Directory = t.Focused.Directory.Foreground(BorgGreen)
	t.Focused.Description = t.Focused.Description.Foreground(MutedGreen)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(AlertRed)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(AlertRed)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(DataCyan)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(DataCyan)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(DataCyan)
	t.Focused.Option = t.Focused.Option.Foreground(Foreground)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(DataCyan)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(HiveGreen).Bold(true)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(BorgGreen).SetString("[•] ")
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(MutedGreen).SetString("[ ] ")
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(Foreground)
	t.Focused.FocusedButton = button.Foreground(lipgloss.Color("#001B0A")).Background(HiveGreen).Bold(true)
	t.Focused.Next = t.Focused.FocusedButton
	t.Focused.BlurredButton = button.Foreground(MutedGreen).Background(DeepGreen)

	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(HiveGreen)
	t.Focused.TextInput.CursorText = t.Focused.TextInput.CursorText.Foreground(Foreground)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(CorpseGreen)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(DataCyan)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description
	return t
}

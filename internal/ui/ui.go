// Package ui provides styled output helpers for CLI feedback using lipgloss.
// All helpers write to an io.Writer and automatically adapt to terminal
// capabilities — color and borders are disabled when the output is not a TTY
// (pipes, CI, direnv subprocesses, etc.).
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// PanelEntry is a single key/source row shown inside a Panel call.
type PanelEntry struct {
	Key    string // variable or item name
	Value  string // display value (may be empty)
	Source string // origin URI, e.g. "bw://folder/item" or "vault://path#field"; empty = literal
}

// rendererFor returns a lipgloss Renderer for w.
// For non-TTY writers it forces the Ascii profile so ANSI codes are stripped.
func rendererFor(w io.Writer) *lipgloss.Renderer {
	if f, ok := w.(*os.File); ok {
		return lipgloss.NewRenderer(f)
	}
	return lipgloss.NewRenderer(w, termenv.WithProfile(termenv.Ascii))
}

// isTTY reports whether w is an *os.File attached to a TTY.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// ── Inline colour helpers ────────────────────────────────────────────────────
// These are writer-aware: colour is stripped for non-TTY writers.

func styledStr(w io.Writer, s string, fn func(*lipgloss.Renderer) lipgloss.Style) string {
	return fn(rendererFor(w)).Render(s)
}

// Bold returns s in bold.
func Bold(w io.Writer, s string) string {
	return styledStr(w, s, func(r *lipgloss.Renderer) lipgloss.Style {
		return r.NewStyle().Bold(true)
	})
}

// Green returns s in green.
func Green(w io.Writer, s string) string {
	return styledStr(w, s, func(r *lipgloss.Renderer) lipgloss.Style {
		return r.NewStyle().Foreground(lipgloss.Color("2"))
	})
}

// Yellow returns s in yellow.
func Yellow(w io.Writer, s string) string {
	return styledStr(w, s, func(r *lipgloss.Renderer) lipgloss.Style {
		return r.NewStyle().Foreground(lipgloss.Color("3"))
	})
}

// Red returns s in red.
func Red(w io.Writer, s string) string {
	return styledStr(w, s, func(r *lipgloss.Renderer) lipgloss.Style {
		return r.NewStyle().Foreground(lipgloss.Color("1"))
	})
}

// Cyan returns s in cyan.
func Cyan(w io.Writer, s string) string {
	return styledStr(w, s, func(r *lipgloss.Renderer) lipgloss.Style {
		return r.NewStyle().Foreground(lipgloss.Color("6"))
	})
}

// Gray returns s in muted gray.
func Gray(w io.Writer, s string) string {
	return styledStr(w, s, func(r *lipgloss.Renderer) lipgloss.Style {
		return r.NewStyle().Foreground(lipgloss.Color("8"))
	})
}

// ── One-line feedback helpers ────────────────────────────────────────────────

// Success prints a green checkmark line to w.
func Success(w io.Writer, msg string) {
	fmt.Fprintln(w, Green(w, "✓")+" "+msg)
}

// Warn prints a yellow warning line to w.
func Warn(w io.Writer, msg string) {
	fmt.Fprintln(w, Yellow(w, "⚠")+" "+msg)
}

// Error prints a red cross line to w.
func Error(w io.Writer, msg string) {
	fmt.Fprintln(w, Red(w, "✗")+" "+msg)
}

// Info prints a cyan bullet line to w.
func Info(w io.Writer, msg string) {
	fmt.Fprintln(w, Cyan(w, "•")+" "+msg)
}

// Header prints a bold section title followed by a separator rule.
func Header(w io.Writer, title string) {
	fmt.Fprintln(w, Bold(w, title))
	fmt.Fprintln(w, Gray(w, strings.Repeat("─", 42)))
}

// Item prints an indented key/value row, key padded to 22 chars.
func Item(w io.Writer, key, value string) {
	fmt.Fprintf(w, "  %-22s %s\n", key, value)
}

// List prints each name as an indented bullet.
func List(w io.Writer, names []string) {
	for _, n := range names {
		fmt.Fprintf(w, "  %s %s\n", Gray(w, "–"), n)
	}
}

// ── Lipgloss panel ───────────────────────────────────────────────────────────

// Panel renders a rounded-border summary box to w.
//
// title is shown in the top border. headline is a one-line summary shown at the
// top of the box body. entries are key/source rows listed below the headline.
//
// When w is not a TTY, a compact plain-text block is printed instead so that
// direnv and other non-interactive contexts get readable output without noise.
func Panel(w io.Writer, title, headline string, entries []PanelEntry) {
	r := rendererFor(w)

	if !isTTY(w) {
		// Compact plain-text: one header line + indented entry list.
		fmt.Fprintf(w, "%s %s\n", Green(w, "✓"), headline)
		for _, e := range entries {
			display := entryDisplay(e)
			fmt.Fprintf(w, "  %-24s %s\n", e.Key, Gray(w, display))
		}
		return
	}

	// ── Styles ──────────────────────────────────────────────────────────────
	borderColor := lipgloss.Color("4") // blue
	titleStyle := r.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("6")) // cyan title

	headlineStyle := r.NewStyle().
		Bold(true)

	keyStyle := r.NewStyle().
		Foreground(lipgloss.Color("4")).  // blue key
		Width(24)

	srcStyle := r.NewStyle().
		Foreground(lipgloss.Color("8")) // gray source

	boxStyle := r.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)

	// ── Build inner content ──────────────────────────────────────────────────
	var sb strings.Builder
	sb.WriteString(Green(w, "✓") + " " + headlineStyle.Render(headline))

	if len(entries) > 0 {
		sb.WriteString("\n")
		for _, e := range entries {
			display := entryDisplay(e)
			line := keyStyle.Render(e.Key) + srcStyle.Render(display)
			sb.WriteString("\n" + line)
		}
	}

	// ── Title in top border ──────────────────────────────────────────────────
	box := boxStyle.Render(sb.String())
	titleRendered := " " + titleStyle.Render(title) + " "

	// Replace the top border's first rune sequence with the title text.
	lines := strings.SplitN(box, "\n", 2)
	if len(lines) == 2 {
		topLine := lines[0]
		// Find where to inject title — after the opening corner char (╭).
		// We replace `─` chars with the title, preserving the border length.
		bare := stripBorderTitle(topLine, titleRendered)
		fmt.Fprintln(w, bare+"\n"+lines[1])
	} else {
		fmt.Fprintln(w, box)
	}
}

// stripBorderTitle replaces leading border dashes after the first corner char
// with title text, so the title appears flush in the top border.
func stripBorderTitle(topLine, title string) string {
	// topLine looks like:  ╭──────────────────────╮
	// We want:             ╭─ title ───────────────╮
	runes := []rune(topLine)
	titleRunes := []rune(title)
	if len(runes) < 3 || len(titleRunes)+3 > len(runes) {
		return topLine
	}
	result := make([]rune, len(runes))
	result[0] = runes[0] // opening corner
	copy(result[1:1+len(titleRunes)], titleRunes)
	// Fill remainder with the border dash character from the original line.
	dash := runes[1]
	for i := 1 + len(titleRunes); i < len(runes)-1; i++ {
		result[i] = dash
	}
	result[len(runes)-1] = runes[len(runes)-1] // closing corner
	return string(result)
}

// sourceLabel formats a source URI into a short display label.
func sourceLabel(source string) string {
	if source == "" {
		return "(literal)"
	}
	return "← " + source
}

// entryDisplay returns the right-hand display string for a PanelEntry.
// - Both Value and Source: "value  ← source"
// - Source only: "← source"
// - Value only: "value"
// - Neither: "(literal)"
func entryDisplay(e PanelEntry) string {
	switch {
	case e.Value != "" && e.Source != "":
		return e.Value + "  ← " + e.Source
	case e.Source != "":
		return "← " + e.Source
	case e.Value != "":
		return e.Value
	default:
		return "(literal)"
	}
}

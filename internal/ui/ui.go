// Package ui provides styled output helpers for CLI feedback using lipgloss.
// Colors are applied automatically when the writer is a TTY and stripped
// for pipes, CI, and direnv subprocesses.
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

// Panel prints a compact summary to w: one header line combining title and
// headline, followed by indented key/source rows. No borders or padding.
// Colors are stripped automatically on non-TTY writers.
func Panel(w io.Writer, title, headline string, entries []PanelEntry) {
	r := rendererFor(w)

	dimStyle := r.NewStyle().Foreground(lipgloss.Color("8"))
	keyStyle := r.NewStyle().Foreground(lipgloss.Color("4")).Width(24)

	// One header line: "✓ renv  Loaded 2 variables from .env"
	fmt.Fprintf(w, "%s %s  %s\n",
		Green(w, "✓"),
		Bold(w, title),
		headline,
	)

	for _, e := range entries {
		fmt.Fprintf(w, "  %s%s\n",
			keyStyle.Render(e.Key),
			dimStyle.Render(entryDisplay(e)),
		)
	}
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

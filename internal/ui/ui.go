// Package ui provides simple ANSI-colored output helpers for CLI feedback.
// All helpers write to an io.Writer and automatically disable color when the
// writer is not a terminal (e.g. pipes, CI environments).
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// ANSI escape sequences.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiGray   = "\033[90m"
)

// isTerminal reports whether w is a *os.File attached to a TTY.
func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

func wrap(w io.Writer, code, s string) string {
	if isTerminal(w) {
		return code + s + ansiReset
	}
	return s
}

// Bold returns s wrapped in bold if w is a terminal.
func Bold(w io.Writer, s string) string { return wrap(w, ansiBold, s) }

// Green returns s in green if w is a terminal.
func Green(w io.Writer, s string) string { return wrap(w, ansiGreen, s) }

// Yellow returns s in yellow if w is a terminal.
func Yellow(w io.Writer, s string) string { return wrap(w, ansiYellow, s) }

// Red returns s in red if w is a terminal.
func Red(w io.Writer, s string) string { return wrap(w, ansiRed, s) }

// Cyan returns s in cyan if w is a terminal.
func Cyan(w io.Writer, s string) string { return wrap(w, ansiCyan, s) }

// Gray returns s in gray if w is a terminal.
func Gray(w io.Writer, s string) string { return wrap(w, ansiGray, s) }

// Success prints a green checkmark line to w.
func Success(w io.Writer, msg string) {
	fmt.Fprintln(w, wrap(w, ansiGreen, "✓")+" "+msg)
}

// Warn prints a yellow warning line to w.
func Warn(w io.Writer, msg string) {
	fmt.Fprintln(w, wrap(w, ansiYellow, "⚠")+" "+msg)
}

// Info prints a cyan bullet line to w.
func Info(w io.Writer, msg string) {
	fmt.Fprintln(w, wrap(w, ansiCyan, "•")+" "+msg)
}

// Header prints a bold section title followed by a separator rule.
func Header(w io.Writer, title string) {
	fmt.Fprintln(w, Bold(w, title))
	fmt.Fprintln(w, Gray(w, strings.Repeat("─", 42)))
}

// Item prints an indented key=value row, with the key padded to 22 chars.
func Item(w io.Writer, key, value string) {
	fmt.Fprintf(w, "  %-22s %s\n", key, value)
}

// List prints each name as an indented gray bullet.
func List(w io.Writer, names []string) {
	for _, n := range names {
		fmt.Fprintf(w, "  %s %s\n", Gray(w, "–"), n)
	}
}

// Package ui provides a progress-aware wrapper for secret resolution.
// This enables spinners and detailed feedback during long operations.
package ui

import (
	"io"
	"log/slog"
	"sync"

	"github.com/opalbolt/envoke/internal/providers"
)

// ProgressRegistry wraps a providers.Registry to provide spinner feedback
// during secret resolution.
type ProgressRegistry struct {
	reg      *providers.Registry
	spinner  *Spinner
	muOnce   sync.Once
	writer   io.Writer
	enabled  bool
}

// NewProgressRegistry creates a registry wrapper that shows spinner feedback.
// If w is nil or not a TTY, the spinner is disabled but resolution still works.
func NewProgressRegistry(reg *providers.Registry, w io.Writer) *ProgressRegistry {
	if reg == nil {
		return &ProgressRegistry{
			reg:     reg,
			writer:  w,
			enabled: false,
		}
	}

	return &ProgressRegistry{
		reg:     reg,
		writer:  w,
		enabled: w != nil,
	}
}

// IsSecretRef delegates to the underlying registry.
func (pr *ProgressRegistry) IsSecretRef(uri string) bool {
	if pr.reg == nil {
		return false
	}
	return pr.reg.IsSecretRef(uri)
}

// Resolve fetches the secret and shows progress feedback.
// The first call to Resolve creates and starts the spinner;
// subsequent calls update the spinner message.
// Close() must be called to stop the spinner.
func (pr *ProgressRegistry) Resolve(uri string) (string, error) {
	if pr.reg == nil {
		return "", nil
	}

	// Start spinner on first Resolve call
	if pr.enabled {
		pr.muOnce.Do(func() {
			pr.spinner = NewSpinner(pr.writer, "Resolving secrets...")
			pr.spinner.Start()
		})
		pr.spinner.SetMessage("Resolving: " + uri)
	}

	slog.Debug("resolving with progress", "uri", uri)
	value, err := pr.reg.Resolve(uri)
	if err != nil {
		slog.Error("resolution failed", "uri", uri, "error", err)
		if pr.spinner != nil {
			pr.spinner.SetMessage("Error resolving: " + uri)
		}
	}
	return value, err
}

// ProviderFor delegates to the underlying registry.
func (pr *ProgressRegistry) ProviderFor(scheme string) (providers.Provider, bool) {
	if pr.reg == nil {
		return nil, false
	}
	return pr.reg.ProviderFor(scheme)
}

// Close stops the spinner and closes the underlying registry.
func (pr *ProgressRegistry) Close() error {
	if pr.spinner != nil {
		pr.spinner.Stop()
	}
	if pr.reg == nil {
		return nil
	}
	return pr.reg.Close()
}

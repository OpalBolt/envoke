package kubeconfig

import (
	"fmt"
	"log/slog"
	"strings"
)

// ValidateKubeconfig checks that the content looks like a valid kubeconfig.
func ValidateKubeconfig(content []byte) error {
	if !strings.Contains(string(content), "apiVersion") {
		return fmt.Errorf("content does not appear to be a kubeconfig (missing 'apiVersion')")
	}
	return nil
}

// WriteKubeconfig writes content to a tmpfile and returns the path.
func WriteKubeconfig(content []byte) (string, error) {
	slog.Debug("validating kubeconfig content", "bytes", len(content))
	if err := ValidateKubeconfig(content); err != nil {
		return "", err
	}
	f, err := NewTempFile("kctx")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(content); err != nil {
		return "", fmt.Errorf("writing kubeconfig: %w", err)
	}
	slog.Info("wrote kubeconfig to tmpfile", "path", f.Name())
	return f.Name(), nil
}

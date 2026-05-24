package env

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/opalbolt/envoke/internal/kubeconfig"
	"github.com/opalbolt/envoke/internal/providers"
	"gopkg.in/yaml.v3"
)

// templateFormat is the extension point for config template formats.
// Register new formats in formatRegistry below.
// Currently only YAML is supported; TOML (#21) and JSON (#22) are planned.
type templateFormat interface {
	// Parse deserialises raw bytes into a generic document tree.
	Parse(data []byte) (interface{}, error)
	// Marshal serialises a resolved document tree back to bytes.
	Marshal(v interface{}) ([]byte, error)
}

// formatRegistry maps lowercase file extensions to their templateFormat handler.
// Add entries here to support new formats (e.g. ".toml", ".json").
var formatRegistry = map[string]templateFormat{
	".yaml": &yamlFormat{},
	".yml":  &yamlFormat{},
}

// formatForPath returns the templateFormat for the given file path based on its
// extension. Returns an error for unsupported extensions.
func formatForPath(path string) (templateFormat, error) {
	ext := strings.ToLower(filepath.Ext(path))
	f, ok := formatRegistry[ext]
	if !ok {
		return nil, fmt.Errorf("config: unsupported template format %q (file: %s)", ext, path)
	}
	return f, nil
}

// yamlFormat implements templateFormat for YAML files.
//
// Note: YAML anchors and aliases are expanded by gopkg.in/yaml.v3 during
// unmarshal. A secret ref under an anchor is resolved once; all aliases see
// the resolved value. This is a known constraint — document anchor-based
// secret refs with care.
type yamlFormat struct{}

func (f *yamlFormat) Parse(data []byte) (interface{}, error) {
	var doc interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("config: parsing YAML: %w", err)
	}
	return doc, nil
}

func (f *yamlFormat) Marshal(v interface{}) ([]byte, error) {
	return yaml.Marshal(v)
}

// RenderConfigTemplate resolves all secret refs in a YAML config template file,
// writes the result to a chmod-600 tmpfile in /dev/shm (falling back to /tmp),
// and returns the tmpfile path.
//
// The tmpfile is prefixed "envoke-render-" and is removed by
// kubeconfig.ClearRendered() on EXIT, screen lock, and sleep.
//
// Errors are returned immediately — a missing template file or an
// unresolvable secret ref aborts the entire resolve.
//
// Note: YAML anchors and aliases are expanded by yaml.v3 during unmarshal.
// A secret ref under an anchor is resolved once; all aliases see the resolved value.
func RenderConfigTemplate(templatePath string, reg *providers.Registry) (string, error) {
	slog.Debug("rendering config template", "path", templatePath)

	data, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("config: reading template %s: %w", templatePath, err)
	}

	format, err := formatForPath(templatePath)
	if err != nil {
		return "", err
	}

	doc, err := format.Parse(data)
	if err != nil {
		return "", err
	}

	resolved, err := walkAndResolve(doc, reg)
	if err != nil {
		return "", fmt.Errorf("config: resolving secrets in %s: %w", templatePath, err)
	}

	out, err := format.Marshal(resolved)
	if err != nil {
		return "", fmt.Errorf("config: marshalling %s: %w", templatePath, err)
	}

	f, err := kubeconfig.NewTempFile("envoke-render")
	if err != nil {
		return "", fmt.Errorf("config: creating tmpfile: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(out); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("config: writing tmpfile: %w", err)
	}

	slog.Debug("rendered config template", "template", templatePath, "tmpfile", f.Name())
	return f.Name(), nil
}

package providers

import (
	"fmt"
	"path/filepath"
	"strings"
)

// RenderFunc is the signature of the function used to render a config template.
// It is injected at construction time to avoid an import cycle between
// internal/providers and internal/env.
type RenderFunc func(templatePath string, reg *Registry) (string, error)

// ConfigProvider resolves config:// URIs by rendering a YAML template file
// with all bw:// and vault:// leaf values resolved through the registry,
// writing the result to a secure tmpfile, and returning the tmpfile path.
//
// Two-step init is required to avoid a circular dependency:
//
//	cp := NewConfigProvider(dotenvDir, env.RenderConfigTemplate)
//	reg.Register(cp)
//	cp.SetRegistry(reg)
type ConfigProvider struct {
	// DotenvDir is the directory of the .env file, used to resolve relative
	// template paths in config:// URIs.
	DotenvDir string
	reg       *Registry
	render    RenderFunc
}

// NewConfigProvider creates a ConfigProvider for the given .env directory.
// render is typically env.RenderConfigTemplate, injected here to avoid an
// import cycle between internal/providers and internal/env.
// Call SetRegistry after registering with the Registry.
func NewConfigProvider(dotenvDir string, render RenderFunc) *ConfigProvider {
	return &ConfigProvider{DotenvDir: dotenvDir, render: render}
}

// SetRegistry wires the back-reference needed to resolve nested secret refs
// inside the rendered template. Must be called after Register.
func (p *ConfigProvider) SetRegistry(reg *Registry) {
	p.reg = reg
}

// Schemes returns the URI schemes handled by this provider.
func (p *ConfigProvider) Schemes() []string {
	return []string{"config"}
}

// Resolve renders a YAML config template and returns the path to the rendered
// tmpfile. The URI must be of the form "config://relative/path/to/template.yaml".
func (p *ConfigProvider) Resolve(uri string) (string, error) {
	const prefix = "config://"
	if !strings.HasPrefix(uri, prefix) {
		return "", fmt.Errorf("config provider: unexpected URI %q", uri)
	}
	rel := strings.TrimPrefix(uri, prefix)
	if rel == "" {
		return "", fmt.Errorf("config provider: empty path in URI %q", uri)
	}

	dotenvDir := p.DotenvDir
	if dotenvDir == "" {
		dotenvDir = "."
	}

	if p.reg == nil {
		return "", fmt.Errorf("config provider: SetRegistry not called — two-step init required")
	}
	// templatePath may escape dotenvDir via ../. This is intentional: the .env
	// file is user-authored and trusted, same as any bw:// or vault:// reference.
	templatePath := filepath.Join(dotenvDir, rel)
	return p.render(templatePath, p.reg)
}

// Close is a no-op; ConfigProvider holds no resources.
func (p *ConfigProvider) Close() error {
	return nil
}

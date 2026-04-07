package providers

import (
	"fmt"
	"strings"
)

// Registry routes secret URIs to the correct Provider by URI scheme.
// Build one registry at startup and pass it to all callers instead of
// threading concrete BWClient / VaultClient types throughout the codebase.
//
// Usage:
//
//	reg := providers.NewRegistry()
//	reg.Register(providers.NewBWProvider(bwClient))
//	reg.Register(providers.NewVaultProvider(vaultClient))
//	value, err := reg.Resolve("bw://folder/item")
type Registry struct {
	providers map[string]Provider // scheme → provider
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a provider for all of its declared schemes.
// Panics if a scheme is already registered — this is a programming error
// caught at startup, not a runtime condition.
func (r *Registry) Register(p Provider) {
	for _, scheme := range p.Schemes() {
		if _, exists := r.providers[scheme]; exists {
			panic(fmt.Sprintf("providers: scheme %q already registered", scheme))
		}
		r.providers[scheme] = p
	}
}

// Resolve dispatches a secret URI to the appropriate provider.
// Returns an error if no provider is registered for the URI scheme.
func (r *Registry) Resolve(uri string) (string, error) {
	scheme, ok := schemeOf(uri)
	if !ok {
		return "", fmt.Errorf("providers: cannot determine scheme of URI %q", uri)
	}
	p, ok := r.providers[scheme]
	if !ok {
		return "", fmt.Errorf("providers: no provider registered for scheme %q (uri: %s)", scheme, uri)
	}
	return p.Resolve(uri)
}

// ProviderFor returns the provider registered for the given scheme.
// The second return value is false if no provider is registered.
func (r *Registry) ProviderFor(scheme string) (Provider, bool) {
	p, ok := r.providers[scheme]
	return p, ok
}

// IsSecretRef returns true if uri uses a scheme registered with this registry.
func (r *Registry) IsSecretRef(uri string) bool {
	scheme, ok := schemeOf(uri)
	if !ok {
		return false
	}
	_, ok = r.providers[scheme]
	return ok
}

// LocalPassword returns the local cache encryption password from the Bitwarden
// provider (if one is registered). This is needed by callers that store
// kubeconfigs via kubeconfig.NamedStore.Put, which requires the same password
// used for BW cache encryption.
//
// Returns "" if no BW provider is registered or the password has not yet been
// set (i.e. no BW resolve has occurred yet).
//
// Uses an anonymous interface assertion to avoid importing the bitwarden sub-package
// (which would create a circular dependency since bw_provider.go already imports it).
func (r *Registry) LocalPassword() string {
	p, ok := r.providers["bw"]
	if !ok {
		return ""
	}
	if lpp, ok := p.(interface{ LocalPassword() string }); ok {
		return lpp.LocalPassword()
	}
	return ""
}

// Close calls Close on every registered provider.
// A non-nil error from one provider does not stop the others from being closed.
// The last non-nil error is returned.
func (r *Registry) Close() error {
	var last error
	for _, p := range r.providers {
		if err := p.Close(); err != nil {
			last = err
		}
	}
	return last
}

// schemeOf extracts the scheme from a URI of the form "scheme://...".
func schemeOf(uri string) (string, bool) {
	idx := strings.Index(uri, "://")
	if idx <= 0 {
		return "", false
	}
	return uri[:idx], true
}

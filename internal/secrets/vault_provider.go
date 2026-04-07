package secrets

// VaultProvider wraps VaultClient to implement the Provider interface.
// URI parsing is handled here so that callers only see the generic Resolve(uri) method.
//
// Standard vault:// URIs must include a #field fragment (e.g. vault://path#key).
// Callers that use vault:// for kubeconfigs without a fragment should normalise
// the URI before calling Resolve (e.g. append "#kubeconfig").
type VaultProvider struct {
	client *VaultClient
}

// NewVaultProvider creates a VaultProvider backed by the given VaultClient.
func NewVaultProvider(client *VaultClient) *VaultProvider {
	return &VaultProvider{client: client}
}

// Schemes returns ["vault"] — the URI scheme handled by this provider.
func (p *VaultProvider) Schemes() []string { return []string{"vault"} }

// Resolve parses the vault:// URI and delegates to the underlying VaultClient.
func (p *VaultProvider) Resolve(uri string) (string, error) {
	ref, err := ParseVaultRef(uri)
	if err != nil {
		return "", err
	}
	return p.client.Resolve(ref)
}

// Close is a no-op for Vault; the token is read from the environment.
func (p *VaultProvider) Close() error { return nil }

// Client returns the underlying VaultClient.
func (p *VaultProvider) Client() *VaultClient {
	return p.client
}

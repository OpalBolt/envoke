package secrets

// BWProvider wraps BWClient to implement the Provider interface.
// URI parsing is handled here so that callers only see the generic Resolve(uri) method.
//
// BWProvider additionally exposes LocalPassword() which is needed by callers that
// store kubeconfigs via kubeconfig.NamedStore.Put using the same encryption password
// as the BW cache.
type BWProvider struct {
	client *BWClient
}

// NewBWProvider creates a BWProvider backed by the given BWClient.
func NewBWProvider(client *BWClient) *BWProvider {
	return &BWProvider{client: client}
}

// Schemes returns ["bw"] — the URI scheme handled by this provider.
func (p *BWProvider) Schemes() []string { return []string{"bw"} }

// Resolve parses the bw:// URI and delegates to the underlying BWClient.
func (p *BWProvider) Resolve(uri string) (string, error) {
	ref, err := ParseBWRef(uri)
	if err != nil {
		return "", err
	}
	return p.client.Resolve(ref)
}

// Close zeros the in-process BW session token.
func (p *BWProvider) Close() error {
	p.client.Close()
	return nil
}

// LocalPassword returns the local cache encryption password held by the underlying
// BWClient. Empty until the first BW Resolve call has prompted (or read from env).
func (p *BWProvider) LocalPassword() string {
	return p.client.LocalPassword
}

// Client returns the underlying BWClient.
// Use this only when you need access to BW-specific operations
// (e.g. FolderItems) that are not part of the Provider interface.
func (p *BWProvider) Client() *BWClient {
	return p.client
}

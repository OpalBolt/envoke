package providers

import (
	bw "github.com/opalbolt/envoke/internal/providers/bitwarden"
)

// BWProvider wraps BWClient to implement the Provider interface.
// URI parsing is handled here so that callers only see the generic Resolve(uri) method.
type BWProvider struct {
	client *bw.BWClient
}

// NewBWProvider creates a BWProvider backed by the given BWClient.
func NewBWProvider(client *bw.BWClient) *BWProvider {
	return &BWProvider{client: client}
}

// Schemes returns ["bw"] — the URI scheme handled by this provider.
func (p *BWProvider) Schemes() []string { return []string{"bw"} }

// Resolve parses the bw:// URI and delegates to the underlying BWClient.
func (p *BWProvider) Resolve(uri string) (string, error) {
	ref, err := bw.ParseBWRef(uri)
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

// Client returns the underlying BWClient.
// Use this only when you need access to BW-specific operations
// (e.g. FolderItems) that are not part of the Provider interface.
func (p *BWProvider) Client() *bw.BWClient {
	return p.client
}

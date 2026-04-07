package secrets

// Provider is the extension point for secret backends.
// Each backend (Bitwarden, Vault, etc.) implements this interface and registers
// itself with a Registry so callers never need to import concrete client types.
type Provider interface {
	// Schemes returns the URI schemes this provider handles, e.g. ["bw"] or ["vault"].
	Schemes() []string

	// Resolve fetches the secret identified by the full URI string (e.g. "bw://folder/item"
	// or "vault://path#field") and returns its plaintext value.
	// URI parsing and validation are the responsibility of the provider.
	Resolve(uri string) (string, error)

	// Close releases any in-process resources (sessions, tokens).
	// It must be safe to call multiple times.
	Close() error
}

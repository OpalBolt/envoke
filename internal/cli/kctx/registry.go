package kctx

import (
	"github.com/eficode/secure-handling-of-secrets/internal/config"
	bw "github.com/eficode/secure-handling-of-secrets/internal/providers/bitwarden"
	"github.com/eficode/secure-handling-of-secrets/internal/providers"
	vlt "github.com/eficode/secure-handling-of-secrets/internal/providers/vault"
	"github.com/eficode/secure-handling-of-secrets/internal/kubeconfig"
)

func newRegistry(cfg *config.Config) *providers.Registry {
	cache := bw.NewCache()
	cache.MaxAge = cfg.CacheMaxAge()
	bwClient := &bw.BWClient{
		Cache:   cache,
		Timeout: cfg.BitwardenTimeout(),
	}
	vaultClient := &vlt.VaultClient{Timeout: cfg.VaultTimeout()}

	reg := providers.NewRegistry()
	reg.Register(providers.NewBWProvider(bwClient))
	reg.Register(providers.NewVaultProvider(vaultClient))
	return reg
}

// normalizeKubeconfigURI delegates to kubeconfig.NormalizeSourceURI.
func normalizeKubeconfigURI(source string) string {
	return kubeconfig.NormalizeSourceURI(source)
}

// kctxLocalPassword returns the local encryption password for the kubeconfig store.
// Prefers the password already established by the BW provider (avoiding a second
// prompt when BW was used). Falls back to a fresh ReadLocalPassword prompt when
// the registry has no BW provider or no BW resolve has occurred (e.g. Vault path).
func kctxLocalPassword(reg *providers.Registry) (string, error) {
	if lp := reg.LocalPassword(); lp != "" {
		return lp, nil
	}
	return bw.ReadLocalPassword()
}

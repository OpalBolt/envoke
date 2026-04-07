package renv

import (
	"github.com/eficode/secure-handling-of-secrets/internal/config"
	bw "github.com/eficode/secure-handling-of-secrets/internal/providers/bitwarden"
	"github.com/eficode/secure-handling-of-secrets/internal/providers"
	vlt "github.com/eficode/secure-handling-of-secrets/internal/providers/vault"
)

func newRegistry(noCache bool, cfg *config.Config) *providers.Registry {
	cache := bw.NewCache()
	cache.MaxAge = cfg.CacheMaxAge()
	if noCache {
		cache.Disabled = true
	}
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

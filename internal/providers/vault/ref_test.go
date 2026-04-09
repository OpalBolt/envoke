package vault_test

import (
	"testing"

	"github.com/eficode/secure-handling-of-secrets/internal/providers/vault"
)

func TestParseVaultRef(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    vault.VaultRef
		wantErr bool
	}{
		{
			name: "basic vault ref",
			uri:  "vault://secret/myapp#password",
			want: vault.VaultRef{Path: "secret/myapp", Field: "password"},
		},
		{
			name: "nested path",
			uri:  "vault://secret/data/myapp/prod#api_key",
			want: vault.VaultRef{Path: "secret/data/myapp/prod", Field: "api_key"},
		},
		{
			name:    "missing field fragment",
			uri:     "vault://secret/myapp",
			wantErr: true,
		},
		{
			name:    "empty field",
			uri:     "vault://secret/myapp#",
			wantErr: true,
		},
		{
			name:    "empty path",
			uri:     "vault://#field",
			wantErr: true,
		},
		{
			name:    "not vault URI",
			uri:     "bw://folder/item",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := vault.ParseVaultRef(tt.uri)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseVaultRef(%q) expected error, got nil", tt.uri)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseVaultRef(%q) unexpected error: %v", tt.uri, err)
				return
			}
			if got != tt.want {
				t.Errorf("ParseVaultRef(%q) = %+v, want %+v", tt.uri, got, tt.want)
			}
		})
	}
}

package secrets

import (
	"testing"
)

func TestParseBWRef(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    BWRef
		wantErr bool
	}{
		{
			name: "basic with default field",
			uri:  "bw://myfolder/myitem",
			want: BWRef{Folder: "myfolder", Item: "myitem", FieldSpec: "password"},
		},
		{
			name: "with username field",
			uri:  "bw://myfolder/myitem/username",
			want: BWRef{Folder: "myfolder", Item: "myitem", FieldSpec: "username"},
		},
		{
			name: "with note field",
			uri:  "bw://myfolder/myitem/note",
			want: BWRef{Folder: "myfolder", Item: "myitem", FieldSpec: "note"},
		},
		{
			name: "with totp field",
			uri:  "bw://myfolder/myitem/totp",
			want: BWRef{Folder: "myfolder", Item: "myitem", FieldSpec: "totp"},
		},
		{
			name: "with custom field",
			uri:  "bw://myfolder/myitem/field:api_key",
			want: BWRef{Folder: "myfolder", Item: "myitem", FieldSpec: "field:api_key"},
		},
		{
			name:    "missing folder",
			uri:     "bw:///myitem",
			wantErr: true,
		},
		{
			name:    "missing item",
			uri:     "bw://myfolder/",
			wantErr: true,
		},
		{
			name:    "not a bw URI",
			uri:     "https://example.com",
			wantErr: true,
		},
		{
			name:    "only folder no item",
			uri:     "bw://myfolder",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBWRef(tt.uri)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseBWRef(%q) expected error, got nil", tt.uri)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseBWRef(%q) unexpected error: %v", tt.uri, err)
				return
			}
			if got != tt.want {
				t.Errorf("ParseBWRef(%q) = %+v, want %+v", tt.uri, got, tt.want)
			}
		})
	}
}

func TestParseVaultRef(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    VaultRef
		wantErr bool
	}{
		{
			name: "basic vault ref",
			uri:  "vault://secret/myapp#password",
			want: VaultRef{Path: "secret/myapp", Field: "password"},
		},
		{
			name: "nested path",
			uri:  "vault://secret/data/myapp/prod#api_key",
			want: VaultRef{Path: "secret/data/myapp/prod", Field: "api_key"},
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
			got, err := ParseVaultRef(tt.uri)
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

func TestIsSecretRef(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"bw://folder/item", true},
		{"vault://path#field", true},
		{"plaintext", false},
		{"https://example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		got := IsSecretRef(tt.input)
		if got != tt.want {
			t.Errorf("IsSecretRef(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

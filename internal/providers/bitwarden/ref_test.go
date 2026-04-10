package bitwarden_test

import (
	"testing"

	"github.com/eficode/envoke/internal/providers/bitwarden"
)

func TestParseBWRef(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    bitwarden.BWRef
		wantErr bool
	}{
		{
			name: "basic with default field",
			uri:  "bw://myfolder/myitem",
			want: bitwarden.BWRef{Folder: "myfolder", Item: "myitem", FieldSpec: "password"},
		},
		{
			name: "with username field",
			uri:  "bw://myfolder/myitem/username",
			want: bitwarden.BWRef{Folder: "myfolder", Item: "myitem", FieldSpec: "username"},
		},
		{
			name: "with note field",
			uri:  "bw://myfolder/myitem/note",
			want: bitwarden.BWRef{Folder: "myfolder", Item: "myitem", FieldSpec: "note"},
		},
		{
			name: "with totp field",
			uri:  "bw://myfolder/myitem/totp",
			want: bitwarden.BWRef{Folder: "myfolder", Item: "myitem", FieldSpec: "totp"},
		},
		{
			name: "with custom field",
			uri:  "bw://myfolder/myitem/field:api_key",
			want: bitwarden.BWRef{Folder: "myfolder", Item: "myitem", FieldSpec: "field:api_key"},
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
			got, err := bitwarden.ParseBWRef(tt.uri)
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

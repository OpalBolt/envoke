package bitwarden

import (
	"testing"
)

func TestExtractFieldAllTypes(t *testing.T) {
	item := map[string]interface{}{
		"name":  "myitem",
		"notes": "some notes",
		"login": map[string]interface{}{
			"password": "pass123",
			"username": "user123",
			"totp":     "totp123",
		},
		"fields": []interface{}{
			map[string]interface{}{"name": "api_key", "value": "key123"},
		},
	}

	tests := []struct {
		spec string
		want string
	}{
		{"password", "pass123"},
		{"username", "user123"},
		{"note", "some notes"},
		{"notes", "some notes"},
		{"totp", "totp123"},
		{"field:api_key", "key123"},
	}

	for _, tt := range tests {
		got, err := extractField(item, tt.spec)
		if err != nil {
			t.Errorf("extractField(%q): %v", tt.spec, err)
			continue
		}
		if got != tt.want {
			t.Errorf("extractField(%q) = %q, want %q", tt.spec, got, tt.want)
		}
	}
}

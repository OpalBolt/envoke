package providers_test

import (
	"errors"
	"testing"

	"github.com/eficode/secure-handling-of-secrets/internal/providers"
)

// mockProvider is a test double that implements Provider.
type mockProvider struct {
	schemes  []string
	data     map[string]string // uri → value
	closeErr error
	closed   bool
}

func newMockProvider(schemes []string, data map[string]string) *mockProvider {
	return &mockProvider{schemes: schemes, data: data}
}

func (m *mockProvider) Schemes() []string { return m.schemes }

func (m *mockProvider) Resolve(uri string) (string, error) {
	if v, ok := m.data[uri]; ok {
		return v, nil
	}
	return "", errors.New("mock: uri not found: " + uri)
}

func (m *mockProvider) Close() error {
	m.closed = true
	return m.closeErr
}

// ── Registry tests ────────────────────────────────────────────────────────────

func TestRegistry_RoutesScheme(t *testing.T) {
	reg := providers.NewRegistry()
	mp := newMockProvider([]string{"fake"}, map[string]string{
		"fake://item": "secret-value",
	})
	reg.Register(mp)

	got, err := reg.Resolve("fake://item")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "secret-value" {
		t.Errorf("got %q, want %q", got, "secret-value")
	}
}

func TestRegistry_MultipleProviders(t *testing.T) {
	reg := providers.NewRegistry()
	reg.Register(newMockProvider([]string{"alpha"}, map[string]string{"alpha://a": "A"}))
	reg.Register(newMockProvider([]string{"beta"}, map[string]string{"beta://b": "B"}))

	for uri, want := range map[string]string{
		"alpha://a": "A",
		"beta://b":  "B",
	} {
		got, err := reg.Resolve(uri)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", uri, err)
		}
		if got != want {
			t.Errorf("Resolve(%q) = %q, want %q", uri, got, want)
		}
	}
}

func TestRegistry_UnknownScheme(t *testing.T) {
	reg := providers.NewRegistry()
	_, err := reg.Resolve("unknown://item")
	if err == nil {
		t.Fatal("expected error for unknown scheme")
	}
}

func TestRegistry_MissingScheme(t *testing.T) {
	reg := providers.NewRegistry()
	_, err := reg.Resolve("no-scheme-here")
	if err == nil {
		t.Fatal("expected error for URI without scheme")
	}
}

func TestRegistry_DuplicateScheme_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate scheme registration")
		}
	}()
	reg := providers.NewRegistry()
	reg.Register(newMockProvider([]string{"dup"}, nil))
	reg.Register(newMockProvider([]string{"dup"}, nil)) // should panic
}

func TestRegistry_Close_CallsAllProviders(t *testing.T) {
	reg := providers.NewRegistry()
	a := newMockProvider([]string{"aa"}, nil)
	b := newMockProvider([]string{"bb"}, nil)
	reg.Register(a)
	reg.Register(b)

	if err := reg.Close(); err != nil {
		t.Fatalf("Close returned unexpected error: %v", err)
	}
	if !a.closed {
		t.Error("provider aa was not closed")
	}
	if !b.closed {
		t.Error("provider bb was not closed")
	}
}

func TestRegistry_ProviderFor(t *testing.T) {
	reg := providers.NewRegistry()
	mp := newMockProvider([]string{"xyz"}, nil)
	reg.Register(mp)

	p, ok := reg.ProviderFor("xyz")
	if !ok {
		t.Fatal("expected ProviderFor to return true")
	}
	if p != mp {
		t.Error("returned wrong provider")
	}

	_, ok = reg.ProviderFor("missing")
	if ok {
		t.Error("expected ProviderFor to return false for missing scheme")
	}
}

func TestRegistry_LocalPassword_NoBWProvider(t *testing.T) {
	reg := providers.NewRegistry()
	if got := reg.LocalPassword(); got != "" {
		t.Errorf("LocalPassword() = %q, want empty string when no BW provider", got)
	}
}

func TestRegistry_IsSecretRef(t *testing.T) {
	reg := providers.NewRegistry()
	reg.Register(newMockProvider([]string{"bw"}, nil))
	reg.Register(newMockProvider([]string{"vault"}, nil))

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
		got := reg.IsSecretRef(tt.input)
		if got != tt.want {
			t.Errorf("IsSecretRef(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

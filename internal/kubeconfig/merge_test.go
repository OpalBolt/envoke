package kubeconfig

import (
	"os"
	"testing"
)

func TestValidateKubeconfig(t *testing.T) {
	valid := []byte("apiVersion: v1\nkind: Config\n")
	if err := ValidateKubeconfig(valid); err != nil {
		t.Errorf("expected valid kubeconfig to pass, got: %v", err)
	}

	invalid := []byte("not a kubeconfig")
	if err := ValidateKubeconfig(invalid); err == nil {
		t.Error("expected invalid kubeconfig to fail")
	}
}

func TestWriteKubeconfig(t *testing.T) {
	content := []byte("apiVersion: v1\nkind: Config\nclusters: []\n")
	path, err := WriteKubeconfig(content)
	if err != nil {
		t.Fatalf("WriteKubeconfig: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch")
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("expected mode 0600, got %v", fi.Mode().Perm())
	}
}

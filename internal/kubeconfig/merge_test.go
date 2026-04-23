package kubeconfig

import (
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

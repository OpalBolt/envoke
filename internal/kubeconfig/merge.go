package kubeconfig

import (
	"fmt"
	"strings"
)

// ValidateKubeconfig checks that the content looks like a valid kubeconfig.
func ValidateKubeconfig(content []byte) error {
	if !strings.Contains(string(content), "apiVersion") {
		return fmt.Errorf("content does not appear to be a kubeconfig (missing 'apiVersion')")
	}
	return nil
}

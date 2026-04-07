package kubeconfig

import "strings"

// NormalizeSourceURI normalises a kubeconfig source URI so it can be passed
// to a providers.Registry. vault:// URIs without a #field fragment default to
// "#kubeconfig". Bare paths (no scheme) are treated as vault:// KV paths.
func NormalizeSourceURI(source string) string {
	if strings.HasPrefix(source, "bw://") {
		return source
	}
	if strings.HasPrefix(source, "vault://") {
		if !strings.Contains(source, "#") {
			return source + "#kubeconfig"
		}
		return source
	}
	return "vault://" + source + "#kubeconfig"
}

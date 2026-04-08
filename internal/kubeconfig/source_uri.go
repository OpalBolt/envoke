package kubeconfig

import "strings"

// NormalizeSourceURI normalises a kubeconfig source URI so it can be passed
// to a providers.Registry. vault:// URIs without a #field fragment default to
// "#kubeconfig". Bare paths (no scheme) are treated as vault:// KV paths.
// URIs with any other scheme are returned unchanged so that the registry can
// report an unsupported-scheme error rather than silently mangling the value.
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
	// If the value already contains a scheme separator ("://") but is not a
	// recognised scheme, return it unchanged so callers get an explicit error.
	if strings.Contains(source, "://") {
		return source
	}
	return "vault://" + source + "#kubeconfig"
}

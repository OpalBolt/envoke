package kubeconfig

// NormalizeSourceURI normalises a kubeconfig source URI so it can be passed
// to a providers.Registry. bw:// URIs are returned unchanged. Any URI with
// a recognised scheme is returned as-is; unknown schemes produce an
// unsupported-scheme error from the registry rather than being silently
// rewritten.
func NormalizeSourceURI(source string) string {
	return source
}

package main

import (
	"os"

	kctx "github.com/eficode/secure-handling-of-secrets/internal/cli/kctx"
)

func main() {
	if err := kctx.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

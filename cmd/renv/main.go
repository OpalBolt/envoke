package main

import (
	"os"

	renv "github.com/eficode/secure-handling-of-secrets/internal/cli/renv"
)

func main() {
	if err := renv.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

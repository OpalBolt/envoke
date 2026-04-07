package renv

import (
	"fmt"
	"log/slog"

	"github.com/eficode/secure-handling-of-secrets/internal/config"
	"github.com/eficode/secure-handling-of-secrets/internal/env"
	"github.com/spf13/cobra"
)

func yamlCmd(cfg *config.Config) *cobra.Command {
	var file string
	var key string

	cmd := &cobra.Command{
		Use:   "yaml [file]",
		Short: "Resolve secret references in a YAML file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				file = args[0]
			}
			if file == "" {
				return fmt.Errorf("--file or positional argument required")
			}
			slog.Debug("running yaml resolve", "file", file, "key", key)
			reg := newRegistry(false, cfg)

			data, err := env.ResolveYAML(file, reg)
			if err != nil {
				return err
			}

			if key != "" {
				val, err := env.YAMLLookup(data, key)
				if err != nil {
					return err
				}
				fmt.Println(val)
				return nil
			}

			out, err := env.MarshalYAML(data)
			if err != nil {
				return err
			}
			fmt.Print(string(out))
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to YAML file")
	cmd.Flags().StringVar(&key, "key", "", "Dot-notation key to extract (e.g. database.password)")
	return cmd
}

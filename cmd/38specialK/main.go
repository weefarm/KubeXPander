// Command 38specialK is the canonical binary name; sk and 38sk are symlinks
// (or copies) of it. Running it is identical to running sk.
//
// Most users invoke the generated shell slugs (kclo, ksys, ...) which
// call back into `sk dispatch`. The long binary name is for `install`,
// `init`, `help`, and other rare commands where the short name isn't worth
// a symlink.
//
// The name "38specialK" riffs on the default 3-8 character length range for
// slug names.
//
// See the package doc for sk (cmd/sk) for full usage; the commands are
// identical.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/weefarm/38specialK/internal/config"
	"github.com/weefarm/38specialK/internal/dispatch"
	"github.com/weefarm/38specialK/internal/install"
)

var (
	cfgPath string
	dryRun  bool
)

func main() {
	root := &cobra.Command{
		Use:   "38specialK",
		Short: "38specialK — Kubernetes namespace slugs with finalizer ops",
		Long: `38specialK reduces wasted keystrokes interacting with Kubernetes.

This is the canonical binary; ` + "`sk`" + ` and ` + "`38sk`" + ` are symlinks to it.
Most users invoke the generated shell slugs (kclo, ksys, ...) which
call back into ` + "`sk dispatch`" + `.

See ` + "`sk --help`" + ` for full usage; the commands are identical.`,
	}

	root.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "path to slugs.yaml (default: ~/.config/sk/slugs.yaml)")
	root.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "print the kubectl command instead of running it")

	root.AddCommand(dispatchCmd(), installCmd(), initCmd(), listCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func dispatchCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "dispatch <slug-name> [args...]",
		Short: "Run the appropriate kubectl command for a slug",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return dispatch.Dispatch(cfg, args[0], args[1:], dispatch.Options{DryRun: dryRun})
		},
		SilenceUsage: true,
	}
	// Everything after the slug name belongs to kubectl, including flags.
	c.Flags().SetInterspersed(false)
	return c
}

func installCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Emit shell functions for all slugs (source from ~/.bashrc)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return install.Emit(cfg, os.Stdout)
		},
		SilenceUsage: true,
	}
}

func initCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "init",
		Short: "Write a starter slugs.yaml with example slugs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := configPath()
			if err != nil {
				return err
			}
			return config.WriteExample(path, force)
		},
		SilenceUsage: true,
	}
	c.Flags().BoolVar(&force, "force", false, "overwrite an existing config file")
	return c
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show the slugs in the loaded config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			fmt.Print(cfg.String())
			fmt.Println("Slugs:")
			for _, name := range cfg.Names() {
				ns, filtered, _ := cfg.ResolveSlug(name)
				if filtered != nil {
					fmt.Printf("  k%s -> %s (grep %q)\n", name, ns, filtered.Grep)
				} else {
					fmt.Printf("  k%s -> %s\n", name, ns)
				}
			}
			fmt.Printf("  k%s -> all-namespaces\n", cfg.AllSlug)
			return nil
		},
		SilenceUsage: true,
	}
}

func loadConfig() (*config.Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	return config.Load(path)
}

func configPath() (string, error) {
	if cfgPath != "" {
		return cfgPath, nil
	}
	return config.DefaultPath()
}

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/typstify/tpix-cli"
	"github.com/typstify/tpix-cli/api"
	"github.com/typstify/tpix-cli/storage"
)

var (
	rootCmd = cobra.Command{
		Use:   "tpix",
		Short: "A tpix command line client used to manage Typst packages",
	}

	cm       *CliConfigManager
	sdk      *tpix.TpixSdk
	pkgCache storage.PackageStore
)

func main() {
	// Load config on startup
	cm = &CliConfigManager{}
	cfg, err := cm.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(exitAuth)
	}

	store, err := storage.NewFsPackageStore(cfg.TypstCachePkgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(exitGeneric)
	}
	pkgCache = store

	httpClient := api.NewHttpClient(cm)
	sdk = tpix.NewTpixSdk(httpClient, pkgCache)

	rootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Emit a single JSON result document on stdout")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return setupOutput(cmd)
	}

	//rootCmd.PersistentFlags().StringVar(&tpixServer, "server", tpixServer, "TPIX server URL")

	rootCmd.AddCommand(loginCmd())
	rootCmd.AddCommand(logoutCmd())
	rootCmd.AddCommand(whoamiCmd())
	rootCmd.AddCommand(newPackageCmd())
	rootCmd.AddCommand(searchPkgCmd())
	rootCmd.AddCommand(getPkgCmd())
	rootCmd.AddCommand(pullCmd())
	rootCmd.AddCommand(depsCmd())
	rootCmd.AddCommand(queryPkgCmd())
	rootCmd.AddCommand(listCachedCmd())
	rootCmd.AddCommand(removeCachedCmd())
	rootCmd.AddCommand(bundleCmd())
	rootCmd.AddCommand(pushCmd())
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(updateCmd())
	rootCmd.AddCommand(cachePathCmd())
	rootCmd.AddCommand(zoteroCmd())

	cmd, err := rootCmd.ExecuteC()
	if err != nil {
		emitError(cmd, err)
		os.Exit(exitCodeFor(err))
	}
}

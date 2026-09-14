package main

import (
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
		panic(err)
	}

	store, err := storage.NewFsPackageStore(cfg.TypstCachePkgPath)
	if err != nil {
		panic(err)
	}
	pkgCache = store

	httpClient := api.NewHttpClient(cm)
	sdk = tpix.NewTpixSdk(httpClient, pkgCache)
	sdk.WithReporter(cmdReporter)

	//rootCmd.PersistentFlags().StringVar(&tpixServer, "server", tpixServer, "TPIX server URL")

	rootCmd.AddCommand(loginCmd())
	rootCmd.AddCommand(newPackageCmd())
	rootCmd.AddCommand(searchPkgCmd())
	rootCmd.AddCommand(getPkgCmd())
	rootCmd.AddCommand(pullCmd())
	rootCmd.AddCommand(queryPkgCmd())
	rootCmd.AddCommand(listCachedCmd())
	rootCmd.AddCommand(removeCachedCmd())
	rootCmd.AddCommand(bundleCmd())
	rootCmd.AddCommand(pushCmd())
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(updateCmd())
	rootCmd.AddCommand(cachePathCmd())
	rootCmd.AddCommand(zoteroCmd())

	rootCmd.Execute()
}

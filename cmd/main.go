package main

import (
	"github.com/spf13/cobra"
	"github.com/typstify/tpix-cli"
	"github.com/typstify/tpix-cli/api"
)

var (
	rootCmd = cobra.Command{
		Use:   "tpix",
		Short: "A tpix command line client used to manage Typst packages",
	}

	cm  *CliConfigManager
	sdk *tpix.TpixSdk
)

func main() {
	// Load config on startup
	cm = &CliConfigManager{}
	cfg, err := cm.Load()
	if err != nil {
		panic(err)
	}

	if cfg.ApiKey != "" {
		httpClient := api.NewHttpClient(cm)
		sdk = tpix.NewTpixSdk(httpClient)
		sdk.WithReporter(cmdReporter)
	}

	//rootCmd.PersistentFlags().StringVar(&tpixServer, "server", tpixServer, "TPIX server URL")

	rootCmd.AddCommand(loginCmd())
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

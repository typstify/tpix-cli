package main

import (
	"github.com/spf13/cobra"
	"github.com/typstify/tpix-cli/api"
	"github.com/typstify/tpix-cli/config"
)

var (
	rootCmd = cobra.Command{
		Use:   "tpix",
		Short: "A tpix command line client used to manage Typst packages",
	}
)

func main() {
	// Load config on startup
	config.Load()

	api.Init(config.CliCredentialProvider{})

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

	rootCmd.Execute()
}

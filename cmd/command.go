package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	cli "github.com/typstify/tpix-cli"
	"github.com/typstify/tpix-cli/config"
	"github.com/typstify/tpix-cli/version"
)

var cmdReporter = func(msg string) {
	fmt.Print(msg)
}

func loginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login the tpix server",
		Long:  "Login the tpix server. User is required to login for all other operations",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			deviceResp, err := cli.StartLogin()
			if err != nil {
				fmt.Printf("Login failed: %v\n", err)
				return err
			}

			// Display instructions to user
			cmdReporter("=== Device Authorization ===\n")
			cmdReporter(fmt.Sprintf("Visit: %s\n", deviceResp.VerificationURI))
			cmdReporter(fmt.Sprintf("Enter code: %s\n", deviceResp.UserCode))
			cmdReporter(fmt.Sprintf("Code expires in %d seconds\n", deviceResp.ExpiresIn))
			cmdReporter("If the browser does not open, please open the above URL manually.")

			tokenResp, err := cli.PollLoginResult(deviceResp.DeviceCode, deviceResp.ExpiresIn, cmdReporter)
			if err != nil {
				fmt.Printf("Login failed: %v\n", err)
				return err
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			cfg.AccessToken = tokenResp.AccessToken
			cfg.RefreshToken = tokenResp.RefreshToken
			config.Save(cfg)
			cmdReporter("\n\nSuccess! Access token saved\n")

			return nil
		},
	}

	return cmd
}

// searchPkgCmd searches Typst packages from TPIX server.
func searchPkgCmd() *cobra.Command {
	var namespace string
	var kind string
	var category string
	var sort string
	var limit int
	var verbose bool

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for Typst packages",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]

			result, err := cli.SearchPackages(namespace, query, kind, category, sort, limit)
			if err != nil {
				fmt.Printf("failed to search packages: %v", err)
				return nil
			}

			fmt.Printf("Found %d results for '%s':\n\n", result.Count, query)
			for _, r := range result.Results {
				cmdReporter(fmt.Sprintf("@%s/%s - %s\n", r.Namespace, r.Name, r.Description))
				if verbose {
					cmdReporter(fmt.Sprintf("  template: %t\n", r.IsTemplate))
					cmdReporter(fmt.Sprintf("  version: %s\n", r.LatestVersion))
					if len(r.Authors) > 0 {
						cmdReporter(fmt.Sprintf("  authors: %s\n", strings.Join(r.Authors, ", ")))
					}
					if len(r.Categories) > 0 {
						cmdReporter(fmt.Sprintf("  categories: %s\n", strings.Join(r.Categories, ", ")))
					}
					if len(r.Disciplines) > 0 {
						cmdReporter(fmt.Sprintf("  disciplines: %s\n", strings.Join(r.Disciplines, ", ")))
					}
					if r.License != "" {
						cmdReporter(fmt.Sprintf("  license: %s\n", r.License))
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().StringVarP(&kind, "kind", "k", "all", "Filter by package kind, possible values: all (default), pkg, template")
	cmd.Flags().StringVarP(&category, "category", "c", "", "Filter by category, TPIX accept categories defined by Typst Universe.")
	cmd.Flags().StringVarP(&sort, "sort", "s", "", "Filter by package kind, possible values: name, updated, or popularity (default)")
	cmd.Flags().IntVarP(&limit, "limit", "l", 20, "Limit number of results")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show verbose outputs.")

	return cmd
}

// getPkgCmd download Typst packages from TPIX server.
func getPkgCmd() *cobra.Command {
	var noDeps bool

	cmd := &cobra.Command{
		Use:   "get <namespace/name:version>",
		Short: "Download a package from TPIX server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgSpec := args[0]

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cacheDir := cfg.TypstCachePkgPath
			if cacheDir == "" {
				return fmt.Errorf("typst cache directory not configured")
			}

			_, err = cli.DownloadPackage(pkgSpec, cacheDir, noDeps, cmdReporter)
			return err
		},
	}

	cmd.Flags().BoolVar(&noDeps, "no-deps", false, "Skip fetching transitive dependencies")

	return cmd
}

// pullCmd scans the current project for .typ imports and fetches all dependencies.
func pullCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Fetch all package dependencies for the current project",
		Long: `Scan the current directory recursively for .typ files, discover all
#import "@namespace/name:version" references, and download each package
along with its transitive dependencies.

Use --dry-run to see what would be fetched without downloading anything.`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cacheDir := cfg.TypstCachePkgPath
			if cacheDir == "" {
				return fmt.Errorf("typst cache directory not configured")
			}

			// Scan current directory for .typ imports
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			return cli.DownloadProjectDependencies(cwd, cacheDir, dryRun, cmdReporter)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be fetched without downloading")

	return cmd
}

// listCachedCmd lists locally cached/downloaded packages.
func listCachedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List locally cached packages",
		Long:  "List all packages downloaded and cached in the local package cache",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			cacheDir := cfg.TypstCachePkgPath
			if cacheDir == "" {
				return fmt.Errorf("typst cache directory not configured")
			}

			entries, err := os.ReadDir(cacheDir)
			if err != nil {
				return fmt.Errorf("failed to read cache directory: %w", err)
			}

			var count int
			fmt.Printf("Cached packages in %s:\n\n", cacheDir)

			for _, namespace := range entries {
				if !namespace.IsDir() {
					continue
				}
				namespacePath := filepath.Join(cacheDir, namespace.Name())
				pkgs, err := os.ReadDir(namespacePath)
				if err != nil {
					continue
				}
				for _, pkg := range pkgs {
					if !pkg.IsDir() {
						continue
					}
					pkgPath := filepath.Join(namespacePath, pkg.Name())
					versions, err := os.ReadDir(pkgPath)
					if err != nil {
						continue
					}
					for _, version := range versions {
						if !version.IsDir() {
							continue
						}
						count++
						fmt.Printf("@%s/%s:%s\n", namespace.Name(), pkg.Name(), version.Name())
					}
				}
			}

			fmt.Printf("\nTotal: %d packages\n", count)

			return nil
		},
	}

	return cmd
}

// removeCachedCmd removes a cached package.
func removeCachedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <namespace/name:version>",
		Short: "Remove a cached package",
		Long:  "Remove a locally cached package from the cache directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgSpec := args[0]
			namespace, name, version := cli.ParsePkgSpec(pkgSpec)

			if namespace == "" || name == "" || version == "" {
				return fmt.Errorf("invalid package spec: use format @namespace/name:version")
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("typst cache directory not configured")
			}

			cacheDir := cfg.TypstCachePkgPath
			if cacheDir == "" {
				return fmt.Errorf("typst cache directory not configured")
			}

			pkgDir := filepath.Join(cacheDir, namespace, name, version)

			// Check if the package exists
			info, err := os.Stat(pkgDir)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("package @%s/%s:%s not found in cache", namespace, name, version)
				}
				return fmt.Errorf("failed to check package: %v", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("package @%s/%s:%s is not a directory", namespace, name, version)
			}

			if err := os.RemoveAll(pkgDir); err != nil {
				return fmt.Errorf("failed to remove package: %v", err)
			}

			fmt.Printf("Removed @%s/%s:%s from cache\n", namespace, name, version)
			return nil
		},
	}

	return cmd
}

// queryPkgCmd query package detail from TPIX server.
func queryPkgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <namespace/name>",
		Short: "Show detailed information about a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgSpec := args[0]

			pkg, err := cli.QueryPackage(pkgSpec)
			if err != nil {
				return err
			}

			fmt.Printf("Package: @%s/%s\n\n", pkg.Namespace, pkg.Name)
			fmt.Printf("Description: %s\n", pkg.Description)
			fmt.Printf("Website: %s\n", pkg.HomepageURL)
			fmt.Printf("Repository: %s\n", pkg.RepositoryURL)
			fmt.Printf("License: %s\n", pkg.License)
			// Show authors for latest version
			fmt.Printf("Authors: %s\n", strings.Join(pkg.LatestVersion.Authors, ", "))

			fmt.Printf("\nVersions:\n")
			for _, v := range pkg.Versions {
				fmt.Printf("  %s (Typst: %s)\n", v.Version, v.TypstVersion)
			}

			return nil
		},
	}

	return cmd
}

// bundleCmd creates a Typst package from a directory.
func bundleCmd() *cobra.Command {
	var output string
	var exclude []string

	cmd := &cobra.Command{
		Use:   "bundle <directory>",
		Short: "Create a Typst package from a directory",
		Long: `Create a .tar.gz Typst package from a directory containing a typst.toml manifest.
The directory must contain a valid typst.toml file with required fields:
- package.name
- package.version
- package.entrypoint

Files and directories can be excluded using the --exclude flag or the exclude field in typst.toml.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcDir := args[0]

			finalPath, err := cli.BundlePackage(srcDir, output, exclude)
			if err != nil {
				return err
			}

			fmt.Printf("Package created: %s\n", finalPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (default: <directory>.tar.gz)")
	cmd.Flags().StringSliceVarP(&exclude, "exclude", "e", []string{}, "Additional files/directories to exclude")

	return cmd
}

// pushCmd uploads a package to the TPIX server.
func pushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push <package.tar.gz> <namespace>",
		Short: "Upload a package to the TPIX server",
		Long: `Upload a .tar.gz Typst package to the TPIX server.
The package must be a valid Typst package archive created with the bundle command.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			packagePath := args[0]
			namespace := args[1]

			return cli.PushPackage(packagePath, namespace, cmdReporter)
		},
	}

	return cmd
}

// versionCmd shows the current version and checks for updates.
func versionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long:  "Show the current version of tpix-cli and check for available updates",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("tpix-cli version %s\n", version.FormatedVersion())

			// Check for updates
			updater := &version.Updater{}
			hasUpdate, err := updater.Check()
			if err != nil {
				// Don't fail if update check fails, just warn
				fmt.Printf("\nWarning: could not check for updates: %v\n", err)
				return nil
			}

			if hasUpdate {
				latest, err := updater.Latest()
				if err != nil {
					fmt.Printf("\nWarning: could not get latest version info: %v\n", err)
					return nil
				}
				fmt.Printf("\nA new version is available: %s\n", latest.Version)
				fmt.Printf("Run 'tpix update' to upgrade\n")
			} else {
				fmt.Printf("\nYou are running the latest version.\n")
			}

			return nil
		},
	}

	return cmd
}

// updateCmd upgrades tpix-cli to the latest version.
func updateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update tpix-cli to the latest version",
		Long:  "Download and install the latest version of tpix-cli from GitHub releases",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Checking for updates...")

			updater := &version.Updater{}
			hasUpdate, err := updater.Check()
			if err != nil {
				return fmt.Errorf("failed to check for updates: %w", err)
			}

			if !hasUpdate {
				fmt.Println("You are already running the latest version.")
				return nil
			}

			latest, err := updater.Latest()
			if err != nil {
				return fmt.Errorf("failed to get latest version info: %w", err)
			}

			fmt.Printf("Downloading version %s...\n", latest.Version)

			progress, err := updater.Update()
			if err != nil {
				return fmt.Errorf("failed to update: %w", err)
			}

			// Wait for download to complete
			for ratio := range progress.Progress() {
				// Simple progress indicator
				fmt.Printf("\rDownloading... %.1f%%", ratio*100)
			}
			fmt.Println("\rDownloading... 100%")

			if progress.Err != nil {
				return fmt.Errorf("download failed: %w", progress.Err)
			}

			fmt.Printf("\nSuccessfully updated to version %s\n", latest.Version)

			return nil
		},
	}

	return cmd
}

// cachePathCmd prints the cache directory path.
func cachePathCmd() *cobra.Command {
	var setPath string

	cmd := &cobra.Command{
		Use:   "cache-path",
		Short: "Print or set the cache directory path",
		Long: `Print or set the path where Typst packages are cached.

The cache path can be set via:
  1. The --set flag: tpix cache-path --set /custom/path
  2. The TYPST_PACKAGE_CACHE_PATH environment variable

If neither is set, the default path is used:
  - Linux/macOS: ~/.cache/typst/packages
  - Windows: %LOCALAPPDATA%\typst\packages`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			flagSet := cmd.Flags().Changed("set")

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			if flagSet {
				// Flag was explicitly set
				if setPath == "" {
					// Empty string - clear and let Save() use detected default
					cfg.TypstCachePkgPath = ""
					if err := config.Save(cfg); err != nil {
						return fmt.Errorf("failed to save config: %w", err)
					}
					cfg, _ = config.Load()

					fmt.Printf("Cache path reset to: %s\n", cfg.TypstCachePkgPath)
					return nil
				}

				// Validate path - check if it exists and is a directory
				info, err := os.Stat(setPath)
				if err != nil {
					if os.IsNotExist(err) {
						return fmt.Errorf("path does not exist: %s", setPath)
					}
					return fmt.Errorf("invalid path: %w", err)
				}
				if !info.IsDir() {
					return fmt.Errorf("path is not a directory: %s", setPath)
				}

				cfg.TypstCachePkgPath = setPath
				if err := config.Save(cfg); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}
				cfg, _ = config.Load()

				fmt.Printf("Cache path set to: %s\n", cfg.TypstCachePkgPath)
				return nil
			}

			cacheDir := cfg.TypstCachePkgPath
			if cacheDir == "" {
				return fmt.Errorf("cache directory not configured")
			}
			fmt.Println(cacheDir)
			return nil
		},
	}

	cmd.Flags().StringVar(&setPath, "set", "", "Set a custom cache path")

	return cmd
}

// zoteroCmd is the parent command for zotero operations.
func zoteroCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "zotero",
		Short: "Zotero integration commands",
		Long:  "Commands for managing Zotero library exports",
	}

	cmd.AddCommand(zoteroListCmd())
	cmd.AddCommand(zoteroExportCmd())
	cmd.AddCommand(zoteroDeleteCmd())

	return cmd
}

// zoteroListCmd lists accessible Zotero libraries.
func zoteroListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List accessible Zotero libraries",
		Long:  "List Zotero libraries the user has access to",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			libraries, err := cli.ListZoteroLibraries()
			if err != nil {
				return fmt.Errorf("failed to list libraries: %w", err)
			}

			if len(libraries) == 0 {
				fmt.Println("No accessible Zotero libraries.")
				return nil
			}

			fmt.Printf("Accessible Zotero libraries:\n\n")
			for i, lib := range libraries {
				scope := lib.Namespace
				if scope == "" {
					scope = "(personal)"
				} else {
					scope = "@" + scope
				}
				fmt.Printf("%d. %s (%s)\n", i+1, scope, lib.Scope)
				fmt.Printf("   Library: %s (ID: %d)\n", lib.Library.Name, lib.Library.ID)
				if len(lib.Collections) > 0 {
					fmt.Printf("   Collections:\n")
					for _, col := range lib.Collections {
						fmt.Printf("     - %s\n", col.Name)
					}
				}
				fmt.Println()
			}

			return nil
		},
	}

	return cmd
}

// zoteroExportCmd creates and downloads a Zotero export.
func zoteroExportCmd() *cobra.Command {
	var format string
	var collection string
	var output string
	var libraryID int64
	var libraryType string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Create and download a Zotero export",
		Long: `Create a Zotero export and download the results.
Interactive mode: run without flags to select library and collection.`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			var targetLib *cli.ZoteroLibrary
			var targetCol string

			// If library ID provided, use it directly
			if libraryID > 0 {
				libraries, err := cli.ListZoteroLibraries()
				if err != nil {
					return fmt.Errorf("failed to list libraries: %w", err)
				}

				for _, lib := range libraries {
					if lib.Library.ID == int(libraryID) {
						targetLib = &lib
						break
					}
				}
				if targetLib == nil {
					return fmt.Errorf("library not found: %d", libraryID)
				}
			} else {
				// Interactive selection
				libraries, err := cli.ListZoteroLibraries()
				if err != nil {
					return fmt.Errorf("failed to list libraries: %w", err)
				}

				if len(libraries) == 0 {
					return fmt.Errorf("no accessible libraries")
				}

				fmt.Println("Select a library:")
				for i, lib := range libraries {
					scope := lib.Namespace
					if scope == "" {
						scope = "(personal)"
					} else {
						scope = "@" + scope
					}
					fmt.Printf("  %d. %s - %s\n", i+1, scope, lib.Library.Name)
				}
				fmt.Print("\nLibrary number: ")

				var choice int
				fmt.Scanf("%d", &choice)
				if choice < 1 || choice > len(libraries) {
					return fmt.Errorf("invalid selection")
				}
				targetLib = &libraries[choice-1]

				// Select collection if available
				if len(targetLib.Collections) > 0 {
					fmt.Println("\nSelect a collection:")
					fmt.Printf("  0. (all items)\n")
					for i, col := range targetLib.Collections {
						fmt.Printf("  %d. %s\n", i+1, col.Name)
					}
					fmt.Print("\nCollection number: ")

					var colChoice int
					fmt.Scanf("%d", &colChoice)
					if colChoice < 0 || colChoice > len(targetLib.Collections) {
						return fmt.Errorf("invalid selection")
					}
					if colChoice > 0 {
						targetCol = targetLib.Collections[colChoice-1].Key
					}
				}
			}

			// Use collection flag if provided
			if collection != "" {
				targetCol = collection
			}

			// Use libraryType from selection
			if libraryType == "" {
				libraryType = targetLib.Scope
			}

			if format == "" {
				format = "biblatex"
			}

			// Generate a name for the export
			exportName := fmt.Sprintf("export-%s", targetLib.Library.Name)

			exportID, err := cli.CreateZoteroExport(exportName, targetLib.NamespaceID, libraryType, int64(targetLib.Library.ID), targetCol, format, cmdReporter)
			if err != nil {
				return fmt.Errorf("failed to create export: %w", err)
			}

			// Fetch and write output
			var writer interface{ Write([]byte) (int, error) }
			if output != "" {
				f, err := os.Create(output)
				if err != nil {
					return fmt.Errorf("failed to create output file: %w", err)
				}
				defer f.Close()
				writer = f
			} else {
				writer = os.Stdout
			}

			if err := cli.FetchZoteroExport(exportID, writer); err != nil {
				return fmt.Errorf("failed to fetch export: %w", err)
			}

			// Clean up the export target (treat as ephemeral in tpix-cli)
			if err := cli.DeleteZoteroExport(exportID, nil); err != nil {
				// Non-fatal, just warn
				fmt.Fprintf(os.Stderr, "Warning: failed to clean up export: %v\n", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "", "Export format (biblatex, bibtex)")
	cmd.Flags().StringVarP(&collection, "collection", "c", "", "Collection key")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file")
	cmd.Flags().Int64VarP(&libraryID, "library", "l", 0, "Library ID")
	cmd.Flags().StringVarP(&libraryType, "library-type", "t", "", "Library type (users/groups)")

	return cmd
}

// zoteroDeleteCmd deletes an existing export.
func zoteroDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <export-id>",
		Short: "Delete a Zotero export",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exportID := args[0]

			if err := cli.DeleteZoteroExport(exportID, cmdReporter); err != nil {
				return fmt.Errorf("failed to delete export: %w", err)
			}

			return nil
		},
	}

	return cmd
}

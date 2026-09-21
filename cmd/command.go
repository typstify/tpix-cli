package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	cli "github.com/typstify/tpix-cli"
	"github.com/typstify/tpix-cli/cmd/pkg"
	"github.com/typstify/tpix-cli/deps"
	"github.com/typstify/tpix-cli/version"
)

func loginCmd() *cobra.Command {
	var apiKey string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login the tpix server",
		Long:  "Login the tpix server with an API key issued by https://tpix.typstify.com",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := strings.TrimSpace(apiKey)
			if key == "" {
				return errors.New("missing api key")
			}

			cfg, err := cm.Load()
			if err != nil {
				return err
			}
			cfg.ApiKey = key

			cm.Save(cfg)
			if currentFormat() == formatText {
				fmt.Fprint(cmd.OutOrStdout(), "Success! API key saved.\n")
			}

			return emitResult(cmd, loginResult{Success: true})
		},
	}

	cmd.Flags().StringVarP(&apiKey, "apiKey", "k", "", " API key issued by https://tpix.typstify.com")

	return cmd
}

// logoutCmd removes the stored API key.
func logoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored API key",
		Long:  "Sign out of the TPIX server by removing the locally stored API key",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := cm.Load()
			if err != nil {
				return err
			}

			cfg.ApiKey = ""
			if err := cm.Save(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			if currentFormat() != formatText {
				return emitResult(cmd, logoutResult{Success: true})
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Logged out.")
			return nil
		},
	}

	return cmd
}

// whoamiCmd shows the profile of the authenticated user.
func whoamiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated TPIX user",
		Long:  "Show the TPIX user profile associated with the configured API key",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, err := sdk.GetUserProfile()
			if err != nil {
				return err
			}

			if currentFormat() != formatText {
				return emitResult(cmd, profile)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Username: %s\n", profile.Username)
			fmt.Fprintf(out, "Email: %s\n", profile.Email)
			if len(profile.Namespaces) > 0 {
				fmt.Fprintf(out, "Namespaces:\n")
				for _, ns := range profile.Namespaces {
					fmt.Fprintf(out, "  - %s (%s)\n", ns.Name, ns.Permission)
				}
			}
			return nil
		},
	}

	return cmd
}

func newPackageCmd() *cobra.Command {
	var targetDir string
	var isTemplate bool
	var namespace string

	cmd := &cobra.Command{
		Use:     "new",
		Short:   "Create a new typst package or template",
		Long:    "Create a new typst package or template skeleton project",
		Example: "tpix new -d ~/work -t -n my-namespace my-package-name",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgName := strings.TrimSpace(args[0])
			if pkgName == "" {
				return errors.New("missing package/template name")
			}

			if targetDir == "" {
				return errors.New("Please specify a desitination directory")
			}

			var username, email string
			user, err := sdk.GetUserProfile()
			if err == nil {
				username = user.Username
				email = user.Email
			}

			pkgDir, err := pkg.CreatePkg(targetDir, namespace, pkgName, isTemplate, username, email)
			if err != nil {
				return fmt.Errorf("failed to create package: %w", err)
			}

			if currentFormat() == formatText {
				fmt.Fprintf(cmd.OutOrStdout(), "Success! Package dir: %s\n", pkgDir)
			}

			return emitResult(cmd, newPackageResult{PackageDir: pkgDir})
		},
	}

	cmd.Flags().StringVarP(&targetDir, "dest", "d", ".", "The directory where to create the package or template.")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "preview", "The namespace name the package belongs to.")
	cmd.Flags().BoolVarP(&isTemplate, "template", "t", false, "Create a template package instead of a library package.")

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
			if query == "" {
				return errors.New("missing query")
			}

			result, err := sdk.SearchPackages(namespace, query, kind, category, sort, limit)
			if err != nil {
				return fmt.Errorf("failed to search packages: %w", err)
			}

			if currentFormat() != formatText {
				return emitResult(cmd, result)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Found %d results for '%s':\n\n", result.Count, query)
			for _, r := range result.Results {
				fmt.Fprintf(out, "@%s/%s - %s\n", r.Namespace, r.Name, r.Description)
				if verbose {
					fmt.Fprintf(out, "  template: %t\n", r.IsTemplate)
					fmt.Fprintf(out, "  version: %s\n", r.LatestVersion)
					if len(r.Authors) > 0 {
						fmt.Fprintf(out, "  authors: %s\n", strings.Join(r.Authors, ", "))
					}
					fmt.Fprintf(out, "  published at: %s\n", r.PublishedAt.Format(time.DateOnly))
					if len(r.Categories) > 0 {
						fmt.Fprintf(out, "  categories: %s\n", strings.Join(r.Categories, ", "))
					}
					if len(r.Disciplines) > 0 {
						fmt.Fprintf(out, "  disciplines: %s\n", strings.Join(r.Disciplines, ", "))
					}
					if r.License != "" {
						fmt.Fprintf(out, "  license: %s\n", r.License)
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

			pkgs, err := sdk.DownloadPackage(pkgSpec, noDeps)
			if err != nil {
				return err
			}
			if currentFormat() == formatText {
				return nil
			}

			items := make([]resolvedPackageOutput, 0, len(pkgs))
			for _, p := range pkgs {
				items = append(items, resolvedPackageOutput{
					Namespace: p.Package.Namespace,
					Name:      p.Package.Name,
					Version:   p.Package.Version,
					Cached:    p.Cached,
				})
			}
			return emitResult(cmd, getResult{Requested: pkgSpec, Packages: items, Resolved: len(items)})
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
			// Scan current directory for .typ imports
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			result, err := sdk.DownloadProjectDependencies(cwd, dryRun)
			if err != nil {
				return err
			}
			if currentFormat() == formatText {
				return nil
			}

			direct := make([]resolvedPackageOutput, 0, len(result.Direct))
			for _, d := range result.Direct {
				direct = append(direct, resolvedPackageOutput{
					Namespace: d.Package.Namespace,
					Name:      d.Package.Name,
					Version:   d.Package.Version,
					Cached:    d.Cached,
				})
			}
			pkgs := make([]resolvedPackageOutput, 0, len(result.Packages))
			for _, p := range result.Packages {
				pkgs = append(pkgs, resolvedPackageOutput{
					Namespace: p.Package.Namespace,
					Name:      p.Package.Name,
					Version:   p.Package.Version,
					Cached:    p.Cached,
				})
			}

			return emitResult(cmd, pullResult{
				ProjectDir: result.ProjectDir,
				DryRun:     result.DryRun,
				Direct:     direct,
				Packages:   pkgs,
				Resolved:   len(pkgs),
			})
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be fetched without downloading")

	return cmd
}

// depsCmd resolves and prints the transitive dependency tree of a package.
func depsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deps <namespace/name>",
		Short: "Show the dependency tree of a package",
		Long:  "Resolve and print the transitive dependencies of a package without downloading it.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			graph, err := sdk.ResolveDependencies(args[0])
			if err != nil {
				return err
			}

			if currentFormat() != formatText {
				flat := make([]resolvedPackageOutput, 0, len(graph.Packages))
				for _, p := range graph.Packages {
					flat = append(flat, resolvedPackageOutput{
						Namespace: p.Package.Namespace,
						Name:      p.Package.Name,
						Version:   p.Package.Version,
						Cached:    p.Cached,
					})
				}
				return emitResult(cmd, depsResult{Root: toDependencyNodeOutput(graph.Root), Packages: flat})
			}

			fmt.Fprint(cmd.OutOrStdout(), formatDependencyTree(graph.Root))
			return nil
		},
	}

	return cmd
}

func toDependencyNodeOutput(node *cli.DependencyNode) dependencyNodeOutput {
	out := dependencyNodeOutput{
		Package: packageOutput{
			Namespace: node.Package.Namespace,
			Name:      node.Package.Name,
			Version:   node.Package.Version,
		},
		Cached: node.Cached,
	}
	for _, child := range node.Children {
		out.Children = append(out.Children, toDependencyNodeOutput(child))
	}
	return out
}

func formatDependencyTree(root *cli.DependencyNode) string {
	status := func(node *cli.DependencyNode) string {
		if node.Cached {
			return "cached"
		}
		return "missing"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s [%s]\n", root.Package, status(root))

	var walk func(node *cli.DependencyNode, prefix string)
	walk = func(node *cli.DependencyNode, prefix string) {
		for i, child := range node.Children {
			last := i == len(node.Children)-1
			connector := "├── "
			if last {
				connector = "└── "
			}
			fmt.Fprintf(&b, "%s%s%s [%s]\n", prefix, connector, child.Package, status(child))

			childPrefix := prefix + "│   "
			if last {
				childPrefix = prefix + "    "
			}
			walk(child, childPrefix)
		}
	}
	walk(root, "")

	return b.String()
}

// listCachedCmd lists locally cached/downloaded packages.
func listCachedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List locally cached packages",
		Long:  "List all packages downloaded and cached in the local package cache",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := cm.Load()
			if err != nil {
				return err
			}

			cacheDir := cfg.TypstCachePkgPath
			if cacheDir == "" {
				return fmt.Errorf("typst cache directory not configured")
			}

			pkgs, err := pkgCache.List(nil)
			if err != nil {
				return err
			}

			if currentFormat() != formatText {
				items := make([]cachedPackage, 0, len(pkgs))
				for _, p := range pkgs {
					item := cachedPackage{Namespace: p.Namespace, Name: p.Name, Version: p.Version}
					if path, err := pkgCache.Path(p); err == nil {
						item.Path = path
					}
					items = append(items, item)
				}
				return emitResult(cmd, cachedListResult{CachePath: cacheDir, Packages: items, Total: len(items)})
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Cached packages in %s:\n\n", cacheDir)

			for _, pkg := range pkgs {
				fmt.Fprintf(out, "%s\n", pkg.String())
			}

			fmt.Fprintf(out, "\nTotal: %d packages\n", len(pkgs))

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
			pkgSpec := deps.ParseDependency(args[0])

			if !pkgSpec.IsComplete() {
				return fmt.Errorf("invalid package spec: use format @namespace/name:version")
			}

			exists, err := pkgCache.Has(pkgSpec)
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("package %s not found in cache", pkgSpec)
			}

			if err := pkgCache.Remove(pkgSpec); err != nil {
				return err
			}

			if currentFormat() != formatText {
				return emitResult(cmd, removeResult{Removed: packageOutput{
					Namespace: pkgSpec.Namespace,
					Name:      pkgSpec.Name,
					Version:   pkgSpec.Version,
				}})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s from cache\n", pkgSpec)
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

			pkg, err := sdk.QueryPackage(pkgSpec)
			if err != nil {
				return err
			}

			if currentFormat() != formatText {
				return emitResult(cmd, pkg)
			}

			fmt.Printf("Package: @%s/%s\n\n", pkg.Namespace, pkg.Name)
			fmt.Printf("Description: %s\n", pkg.Description)
			fmt.Printf("Template: %t\n", pkg.IsTemplate)
			fmt.Printf("License: %s\n", pkg.License)
			fmt.Printf("Authors: %s\n", strings.Join(pkg.Authors, ", "))
			fmt.Printf("Categories: %s\n", strings.Join(pkg.Categories, ", "))
			fmt.Printf("Disciplines: %s\n", strings.Join(pkg.Disciplines, ", "))

			if len(pkg.Versions) > 0 {
				latestVer := pkg.Versions[0]

				fmt.Printf("Latest Version:  %s\n", latestVer.Version)
				fmt.Printf("Minimum Typst Version: %s\n", latestVer.MinCompilerVer)

			}

			fmt.Printf("Last Publish: %s\n", pkg.LastPublishedAt.Format(time.DateOnly))
			fmt.Printf("Website: %s\n", pkg.HomepageURL)
			fmt.Printf("Repository: %s\n", pkg.RepositoryURL)

			fmt.Printf("\nVersions:\n")
			for _, v := range pkg.Versions {
				fmt.Printf("  %s (Typst: %s)\n", v.Version, v.MinCompilerVer)
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

			finalPath, err := sdk.BundlePackage(srcDir, output, exclude)
			if err != nil {
				return err
			}

			if currentFormat() != formatText {
				return emitResult(cmd, bundleResult{SourceDir: srcDir, OutputPath: finalPath})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Package created: %s\n", finalPath)
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

			resp, err := sdk.PushPackage(packagePath, namespace)
			if err != nil {
				return err
			}
			if currentFormat() == formatText {
				return nil
			}

			return emitResult(cmd, pushResult{
				Namespace: resp.Namespace,
				Package:   resp.Package,
				Version:   resp.Version,
				SHA256:    resp.SHA256,
				Size:      resp.Size,
				Success:   resp.SHA256 != "",
				Report:    resp.ValidateReport,
			})
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
			updater := &version.Updater{}
			hasUpdate, checkErr := updater.Check()

			res := versionResult{Version: version.Version, SchemaVersion: schemaVersion, HasUpdate: hasUpdate}
			if hasUpdate {
				if latest, err := updater.Latest(); err == nil {
					res.Latest = latest.Version
				}
			}

			if currentFormat() != formatText {
				return emitResult(cmd, res)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "tpix-cli version %s\n", version.FormatedVersion())

			if checkErr != nil {
				// Don't fail if update check fails, just warn
				fmt.Fprintf(out, "\nWarning: could not check for updates: %v\n", checkErr)
				return nil
			}

			if hasUpdate {
				latest, err := updater.Latest()
				if err != nil {
					fmt.Fprintf(out, "\nWarning: could not get latest version info: %v\n", err)
					return nil
				}
				fmt.Fprintf(out, "\nA new version is available: %s\n", latest.Version)
				fmt.Fprintf(out, "Run 'tpix update' to upgrade\n")
			} else {
				fmt.Fprintf(out, "\nYou are running the latest version.\n")
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
			if err := requireTextFormat(cmd, "update"); err != nil {
				return err
			}

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

			cfg, err := cm.Load()
			if err != nil {
				return err
			}
			previous := cfg.TypstCachePkgPath

			if flagSet {
				// Flag was explicitly set
				if setPath == "" {
					// Empty string - clear and let Save() use detected default
					cfg.TypstCachePkgPath = ""
					if err := cm.Save(cfg); err != nil {
						return fmt.Errorf("failed to save config: %w", err)
					}

					effective, err := cm.Load()
					if err != nil {
						return err
					}

					if currentFormat() != formatText {
						return emitResult(cmd, cachePathResult{Path: effective.TypstCachePkgPath, Source: cachePathSource(), Previous: previous})
					}

					fmt.Fprintf(cmd.OutOrStdout(), "Cache path reset to: %s\n", effective.TypstCachePkgPath)
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
				if err := cm.Save(cfg); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}

				effective, err := cm.Load()
				if err != nil {
					return err
				}

				if currentFormat() != formatText {
					return emitResult(cmd, cachePathResult{Path: effective.TypstCachePkgPath, Source: cachePathSource(), Previous: previous})
				}

				if effective.TypstCachePkgPath != setPath {
					fmt.Fprintf(cmd.OutOrStdout(), "Cache path set to: %s (overridden by %s: %s)\n", setPath, cachePathEnv, effective.TypstCachePkgPath)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Cache path set to: %s\n", effective.TypstCachePkgPath)
				}
				return nil
			}

			cacheDir := cfg.TypstCachePkgPath
			if cacheDir == "" {
				return fmt.Errorf("cache directory not configured")
			}
			if currentFormat() != formatText {
				return emitResult(cmd, cachePathResult{Path: cacheDir, Source: cachePathSource()})
			}
			fmt.Fprintln(cmd.OutOrStdout(), cacheDir)
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
			libraries, err := sdk.ListZoteroLibraries()
			if err != nil {
				return fmt.Errorf("failed to list libraries: %w", err)
			}

			if currentFormat() != formatText {
				return emitResult(cmd, zoteroListResult{Libraries: libraries})
			}

			out := cmd.OutOrStdout()
			if len(libraries) == 0 {
				fmt.Fprintln(out, "No accessible Zotero libraries.")
				return nil
			}

			fmt.Fprintf(out, "Accessible Zotero libraries:\n\n")
			for i, lib := range libraries {
				scope := lib.Namespace
				if scope == "" {
					scope = "(personal)"
				} else {
					scope = "@" + scope
				}
				fmt.Fprintf(out, "%d. %s (%s)\n", i+1, scope, lib.Scope)
				fmt.Fprintf(out, "   Library: %s (ID: %d)\n", lib.Library.Name, lib.Library.ID)
				if len(lib.Collections) > 0 {
					fmt.Fprintf(out, "   Collections:\n")
					for _, col := range lib.Collections {
						fmt.Fprintf(out, "     - %s\n", col.Name)
					}
				}
				fmt.Fprintln(out)
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

			// Machine mode cannot prompt, so require an explicit library.
			if currentFormat() != formatText && libraryID <= 0 {
				return usageErrorf("--library is required in --json mode")
			}

			// If library ID provided, use it directly
			if libraryID > 0 {
				libraries, err := sdk.ListZoteroLibraries()
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
				libraries, err := sdk.ListZoteroLibraries()
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

			exportID, err := sdk.CreateZoteroExport(exportName, targetLib.NamespaceID, libraryType, int64(targetLib.Library.ID), targetCol, format)
			if err != nil {
				return fmt.Errorf("failed to create export: %w", err)
			}

			// Fetch output. When no output file is requested we buffer the content
			// so machine mode can return it as a JSON field instead of writing it
			// to stdout.
			var content []byte
			if output != "" {
				f, err := os.Create(output)
				if err != nil {
					return fmt.Errorf("failed to create output file: %w", err)
				}
				if err := sdk.FetchZoteroExport(exportID, f); err != nil {
					f.Close()
					return fmt.Errorf("failed to fetch export: %w", err)
				}
				if err := f.Close(); err != nil {
					return fmt.Errorf("failed to write output file: %w", err)
				}
			} else {
				var buf bytes.Buffer
				if err := sdk.FetchZoteroExport(exportID, &buf); err != nil {
					return fmt.Errorf("failed to fetch export: %w", err)
				}
				content = buf.Bytes()
			}

			// Clean up the export target (treat as ephemeral in tpix-cli)
			if err := sdk.DeleteZoteroExport(exportID); err != nil {
				// Non-fatal, just warn
				fmt.Fprintf(os.Stderr, "Warning: failed to clean up export: %v\n", err)
			}

			if currentFormat() != formatText {
				res := zoteroExportResult{ExportID: exportID, Format: format, OutputPath: output}
				if output == "" {
					res.Content = string(content)
				}
				return emitResult(cmd, res)
			}

			if output == "" {
				if _, err := cmd.OutOrStdout().Write(content); err != nil {
					return err
				}
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

			if err := sdk.DeleteZoteroExport(exportID); err != nil {
				return fmt.Errorf("failed to delete export: %w", err)
			}

			if currentFormat() != formatText {
				return emitResult(cmd, zoteroDeleteResult{Deleted: exportID})
			}

			return nil
		},
	}

	return cmd
}

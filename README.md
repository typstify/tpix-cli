# TPIX CLI

A command-line client for managing Typst packages on the [TPIX](https://tpix.typstify.com) server. 
For more information about TPIX and TPIX CLI, please visit [TPIX](https://tpix.typstify.com/docs).

## Installation

Use the install script to download and install the latest stable version if you are using Linux or MacOS:

```bash
curl -fsSL https://tpix.typstify.com/install.sh | bash
```

You can also download pre-built binaries from the [GitHub Releases](https://github.com/typstify/tpix-cli/releases) page.

For developers, use the following command to install tpix-cli:

```bash
go install github.com//tpix-cli@latest
```

Or build from source:

```bash
git clone https://github.com/typstify/tpix-cli.git
cd tpix-cli
make install
```

The binary will be built as `tpix` and installed in `/usr/local/bin/` in Linux and MacOS.


## Quick Start

```bash
# Login to TPIX server
tpix login

# Search for packages
tpix search "chart"

# Download a package
tpix get @namespace/package-name:1.0.0

# View package info
tpix info @namespace/package-name
```

## Commands

### Authentication

```bash
tpix login
```

Login using OAuth 2.0 device flow. Required for uploading packages.

### Configuration

```bash
# Print cache directory path
tpix cache-path

# Set custom cache directory
tpix cache-path --set /custom/path

# Reset to default cache directory
tpix cache-path --set ""
```

The cache directory can also be set via the `TYPST_PACKAGE_CACHE_PATH` environment variable, which takes precedence over the saved config value.


### Search & Discovery

```bash
# Search packages
tpix search "chart"

# Search in specific namespace
tpix search "chart" -n mynamespace

# Limit results
tpix search "chart" -l 10
```

### Download Packages

```bash
# Download latest version (with transitive dependencies)
tpix get @namespace/package-name

# Download specific version
tpix get @namespace/package-name:1.0.0

# Download without fetching dependencies
tpix get @namespace/package-name:1.0.0 --no-deps
```

### Pull Project Dependencies

```bash
# Scan current directory for .typ imports and fetch all dependencies
tpix pull

# Preview what would be fetched without downloading
tpix pull --dry-run
```

`tpix pull` recursively scans all `.typ` files in the current directory for `#import "@namespace/name:version"` statements, then downloads each package along with its transitive dependencies. Already-cached packages are skipped.

### Package Info

```bash
# View package details
tpix info @namespace/package-name
```

### Local Cache

```bash
# List cached packages
tpix list

# Remove cached package
tpix remove @namespace/package-name:1.0.0
```

### Create Package

```bash
# Create package from directory
tpix bundle ./my-package

# Specify output file
tpix bundle ./my-package -o my-package.tar.gz

# Exclude files
tpix bundle ./my-package -e ".git" -e "node_modules/" -e "*.test"
```

The directory must contain a valid `typst.toml` manifest with required fields:

```toml
[package]
name = "mypackage"
version = "1.0.0"
entrypoint = "lib.typ"
authors = ["Your Name"]
license = "MIT"
description = "A sample package"
```

You can also specify excluded files in the manifest:

```toml
[package]
# ... other fields
exclude = [".git", "*.test", "node_modules/"]
```

For more information on how to create a package, please refer to docs in https://github.com/typst/packages/tree/main/docs.

### Upload Package

```bash
# Upload to namespace
tpix push my-package.tar.gz mynamespace
```

Requires login first.

### Version & Updates

```bash
# Check current version and updates
tpix version

# Update to latest version
tpix update
```


## Output Format

Package specifications use the format `@namespace/name:version`:

- `@user/chart` - latest version from user's namespace
- `@user/chart:1.0.0` - specific version

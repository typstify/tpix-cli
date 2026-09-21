package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/typstify/tpix-cli/api"
)

// schemaVersion is the version of the `--json` output schema. It is
// emitted on every result/error document so consumers can detect incompatible
// changes.
const schemaVersion = 1

// outputFormat is the effective output format.
type outputFormat string

const (
	formatText outputFormat = "text"
	formatJSON outputFormat = "json"
)

// jsonFlag is bound to the persistent --json flag. A boolean is used instead
// of a format enum because `zotero export` already owns `--format` for the
// export format, which would shadow a global flag of the same name.
var jsonFlag bool

func currentFormat() outputFormat {
	if jsonFlag {
		return formatJSON
	}
	return formatText
}

// Exit codes shared with machine consumers. They are part of the schema.
const (
	exitOK         = 0
	exitGeneric    = 1
	exitUsage      = 2
	exitAuth       = 3
	exitNotFound   = 4
	exitNetwork    = 5
	exitValidation = 6
)

// usageError marks an error caused by invalid CLI usage (bad flags/args).
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

func usageErrorf(format string, args ...any) error {
	return &usageError{err: fmt.Errorf(format, args...)}
}

// requireTextFormat returns a usage error for commands that have not yet been
// converted to machine-readable output.
func requireTextFormat(cmd *cobra.Command, command string) error {
	if !jsonFlag {
		return nil
	}
	return usageErrorf("%s does not support --json yet", command)
}

func exitCodeFor(err error) int {
	if err == nil {
		return exitOK
	}

	var ue *usageError
	if errors.As(err, &ue) {
		return exitUsage
	}

	var reqErr *api.RequestError
	if errors.As(err, &reqErr) {
		return exitCodeForHTTP(reqErr.Code)
	}

	return exitGeneric
}

func exitCodeForHTTP(status int) int {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return exitAuth
	case status == http.StatusNotFound:
		return exitNotFound
	case status == http.StatusBadRequest:
		return exitUsage
	case status == http.StatusUnprocessableEntity:
		return exitValidation
	case status >= 500:
		return exitNetwork
	default:
		return exitGeneric
	}
}

// --- output envelopes ---

// resultLine is the terminal success document.
type resultLine struct {
	SchemaVersion int    `json:"schemaVersion"`
	Type          string `json:"type"`
	Command       string `json:"command"`
	Data          any    `json:"data,omitempty"`
}

// errorLine is the terminal failure document.
type errorLine struct {
	SchemaVersion int          `json:"schemaVersion"`
	Type          string       `json:"type"`
	Command       string       `json:"command"`
	Error         *errorDetail `json:"error"`
}

type errorDetail struct {
	Code        int    `json:"code"`
	HTTPStatus  int    `json:"httpStatus,omitempty"`
	Message     string `json:"message"`
	Description string `json:"description,omitempty"`
}

func commandName(cmd *cobra.Command) string {
	if cmd == nil {
		return "tpix"
	}
	name := strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), "tpix"))
	if name == "" {
		return "tpix"
	}
	return name
}

// cachePathSource reports whether the effective cache path comes from the
// TYPST_PACKAGE_CACHE_PATH environment override or the saved config.
func cachePathSource() string {
	if os.Getenv(cachePathEnv) != "" {
		return "env"
	}
	return "config"
}

// setupOutput wires the SDK's human-readable progress reporter. Progress is
// never part of the machine contract: in JSON mode it is redirected to stderr
// so stdout carries only the single result document.
func setupOutput(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	if jsonFlag {
		out = cmd.ErrOrStderr()
	}
	sdk.WithReporter(func(msg string) { fmt.Fprint(out, msg) })

	if jsonFlag {
		root := cmd.Root()
		root.SilenceErrors = true
		root.SilenceUsage = true
	}
	return nil
}

// emitResult writes the terminal result document when the selected format is
// machine-readable. In text mode it is a no-op; callers render text themselves.
func emitResult(cmd *cobra.Command, data any) error {
	if !jsonFlag {
		return nil
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	return enc.Encode(resultLine{
		SchemaVersion: schemaVersion,
		Type:          "result",
		Command:       commandName(cmd),
		Data:          data,
	})
}

// emitError writes the terminal error document when the selected format is
// machine-readable. In text mode cobra has already printed the error.
func emitError(cmd *cobra.Command, err error) {
	if err == nil || !jsonFlag {
		return
	}

	detail := &errorDetail{Code: exitCodeFor(err), Message: err.Error()}

	var reqErr *api.RequestError
	if errors.As(err, &reqErr) {
		detail.HTTPStatus = reqErr.Code
		if reqErr.Message != "" {
			detail.Message = reqErr.Message
		}
		detail.Description = reqErr.Description
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	_ = enc.Encode(errorLine{
		SchemaVersion: schemaVersion,
		Type:          "error",
		Command:       commandName(cmd),
		Error:         detail,
	})
}

// --- per-command result payloads ---

type loginResult struct {
	Success bool `json:"success"`
}

type logoutResult struct {
	Success bool `json:"success"`
}

type newPackageResult struct {
	PackageDir string `json:"packageDir"`
}

type packageOutput struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version"`
}

type resolvedPackageOutput struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Cached    bool   `json:"cached"`
}

type getResult struct {
	Requested string                  `json:"requested"`
	Packages  []resolvedPackageOutput `json:"packages"`
	Resolved  int                     `json:"resolved"`
}

type pullResult struct {
	ProjectDir string                  `json:"projectDir"`
	DryRun     bool                    `json:"dryRun"`
	Direct     []resolvedPackageOutput `json:"direct"`
	Packages   []resolvedPackageOutput `json:"packages"`
	Resolved   int                     `json:"resolved"`
}

type dependencyNodeOutput struct {
	Package  packageOutput          `json:"package"`
	Cached   bool                   `json:"cached"`
	Children []dependencyNodeOutput `json:"children,omitempty"`
}

type depsResult struct {
	Root     dependencyNodeOutput    `json:"root"`
	Packages []resolvedPackageOutput `json:"packages"`
}

type pushResult struct {
	Namespace string   `json:"namespace"`
	Package   string   `json:"package"`
	Version   string   `json:"version"`
	SHA256    string   `json:"sha256,omitempty"`
	Size      int64    `json:"size,omitempty"`
	Success   bool     `json:"success"`
	Report    []string `json:"report,omitempty"`
}

type cachedPackage struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Path      string `json:"path,omitempty"`
}

type cachedListResult struct {
	CachePath string          `json:"cachePath"`
	Packages  []cachedPackage `json:"packages"`
	Total     int             `json:"total"`
}

type removeResult struct {
	Removed packageOutput `json:"removed"`
}

type bundleResult struct {
	SourceDir  string `json:"sourceDir"`
	OutputPath string `json:"outputPath"`
}

type versionResult struct {
	Version       string `json:"version"`
	SchemaVersion int    `json:"schemaVersion"`
	HasUpdate     bool   `json:"hasUpdate"`
	Latest        string `json:"latest,omitempty"`
}

type cachePathResult struct {
	Path     string `json:"path"`
	Source   string `json:"source"`
	Previous string `json:"previous,omitempty"`
}

type zoteroListResult struct {
	Libraries []api.ZoteroLibrary `json:"libraries"`
}

type zoteroDeleteResult struct {
	Deleted string `json:"deleted"`
}

type zoteroExportResult struct {
	ExportID   string `json:"exportId"`
	Format     string `json:"format"`
	OutputPath string `json:"outputPath,omitempty"`
	Content    string `json:"content,omitempty"`
}

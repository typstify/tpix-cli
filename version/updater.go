package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const (
	latestReleaseUrl = "https://api.github.com/repos/typstify/tpix-cli/releases/latest"
)

type GithubRelease struct {
	ID          int64     `json:"id"`
	URL         string    `json:"html_url"`
	TagName     string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	Body        string    `json:"body"`
	Assets      []Asset   `json:"assets"`
}

type Asset struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Size        int    `json:"size"`
	DownloadURL string `json:"browser_download_url"`
}

type Updater struct {
	latestRelease *Release
}

type Release struct {
	Asset
	Version     string
	Changelog   string
	PublishedAt time.Time
}

// Check queries che GitHub release API to see if there is a new
// release compared to the current version of tpix-cli.
func (u *Updater) Check() (bool, error) {
	if u.latestRelease == nil {
		r, err := u.getRelease()
		if err != nil {
			return false, err
		}

		u.latestRelease = r
	}

	return compareVersion(u.latestRelease.Version, Version)

}

func (u *Updater) Latest() (*Release, error) {
	if u.latestRelease == nil {
		r, err := u.getRelease()
		if err != nil {
			return nil, err
		}

		u.latestRelease = r
	}

	return u.latestRelease, nil
}

// Update downloads the specified version to disk and replace the
// current version.
func (u *Updater) Update() (*DownloadProgress, error) {

	if u.latestRelease == nil {
		return nil, fmt.Errorf("Check if there is a new version first!")
	}

	// Download to temp directory first, then move to final location
	// This avoids issues with replacing the running executable
	tempDir, err := os.MkdirTemp("", "tpix-update-*")
	if err != nil {
		return nil, err
	}

	dl := newDownloader(u.latestRelease.Asset, tempDir)

	progress := dl.Download(func() {
		onDownloadFinished(tempDir)
		os.RemoveAll(tempDir)
	})

	return progress, nil
}

func (d *Updater) getRelease() (*Release, error) {
	// Get release meta from Github API
	resp, err := http.Get(latestReleaseUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var release GithubRelease
	err = decoder.Decode(&release)
	if err != nil {
		return nil, err
	}

	// asset name should be like 'tpix-cli-windows-amd64.tar.gz'
	assetNamePat := fmt.Sprintf(`^tpix-cli-%s-%s-?\w*?\.tar\.gz$`, runtime.GOOS, runtime.GOARCH)
	//log.Println("re pattern: ", assetNamePat)
	re := regexp.MustCompile(assetNamePat)
	var target Asset
	for _, asset := range release.Assets {
		if re.Match([]byte(asset.Name)) {
			target = asset
			break
		}
	}

	if target == (Asset{}) {
		return nil, fmt.Errorf("No matched release for %s-%s", runtime.GOOS, runtime.GOARCH)
	}

	return &Release{
		Asset:       target,
		Version:     release.TagName,
		Changelog:   release.Body,
		PublishedAt: release.PublishedAt,
	}, nil
}

func onDownloadFinished(tempDir string) {
	binaryName := "tpix"
	if runtime.GOOS == "windows" {
		binaryName = "tpix.exe"
	}

	newBinPath := filepath.Join(tempDir, binaryName)
	exePath, err := os.Executable()
	if err != nil {
		return
	}

	// Ensure the new file is executable
	os.Chmod(newBinPath, 0755)

	if runtime.GOOS == "windows" {
		// Windows locks the running executable file, rename current to .old, then move new to current
		oldPath := exePath + ".old"
		os.Remove(oldPath)

		if err := os.Rename(exePath, oldPath); err != nil {
			fmt.Printf("Windows: Failed to rename running exe: %v\n", err)
			return
		}
	}

	// Use a robust move/copy
	err = moveFile(newBinPath, exePath)
	if err != nil {
		fmt.Printf("Failed to replace binary: %v\n", err)
		return
	}
}

func moveFile(src, dst string) error {
	//  Remove the existing binary first to avoid "text file busy"
	// Even if it's running, removing it unlinks the name from the inode.
	_ = os.Remove(dst)

	// Try rename first (atomic and fast)
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// Fallback to manual copy (handles cross-partition moves)
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0755)
}

// compareVersion compares two semantic version string to see
// if v1 is newer than v2. It returns true if v1 is newer than
// v2, otherwise it returns false.
func compareVersion(v1, v2 string) (bool, error) {
	ver1, err := normVersion(v1)
	if err != nil {
		return false, err
	}
	ver2, err := normVersion(v2)
	if err != nil {
		return false, err
	}

	return semver.Compare(ver1, ver2) > 0, nil
}

func normVersion(ver string) (string, error) {
	if ver == "" {
		return "", fmt.Errorf("version cannot be empty")
	}

	if !strings.HasPrefix(ver, "v") {
		ver = "v" + ver
	}

	if !semver.IsValid(ver) {
		return "", fmt.Errorf("invalid semantic version: %s", ver)
	}

	return ver, nil
}

package version

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/typstify/tpix-cli/utils"
)

// DownloadCounter counts the number of bytes written to it. It implements to the io.Writer interface
// and we can pass this into io.TeeReader() which will report progress on each write cycle.
type DownloadProgress struct {
	finished   atomic.Uint64
	total      uint64
	reportChan chan float32
	Err        error
}

func (dp *DownloadProgress) Write(p []byte) (int, error) {
	n := len(p)
	dp.finished.Add(uint64(n))

	// compute progress
	progress := float32(dp.finished.Load()) / float32(dp.total)
	dp.reportChan <- progress
	return n, nil
}

func (dp *DownloadProgress) Progress() chan float32 {
	return dp.reportChan
}

func (dp *DownloadProgress) Done() {
	close(dp.reportChan)
}

// Downloader check and download the latest version of TPIX CLI.
type Downloader struct {
	asset   Asset
	destDir string
	client  *http.Client
}

func newDownloadProgress(total uint64) *DownloadProgress {
	return &DownloadProgress{
		total:      total,
		reportChan: make(chan float32, 5),
	}
}

func newDownloader(asset Asset, destDir string) *Downloader {
	if asset.DownloadURL == "" {
		return nil
	}

	c := &http.Client{
		Timeout: 10 * time.Minute,
	}

	return &Downloader{
		client:  c,
		asset:   asset,
		destDir: destDir,
	}

}

func (d *Downloader) get(url string) (*http.Response, error) {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return d.client.Do(request)
}

// Download downloads the release file in async manner, and reports its progress.
func (d *Downloader) Download(onFinished func()) *DownloadProgress {
	progress := newDownloadProgress(uint64(d.asset.Size))

	go func() {
		defer progress.Done()
		// download the asset
		resp, err := d.get(d.asset.DownloadURL)
		if err != nil {
			progress.Err = err
			return
		}

		var targetFile *os.File
		targetFile, err = os.OpenFile(filepath.Join(d.destDir, d.asset.Name), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			progress.Err = err
			return
		}

		defer targetFile.Close()

		if n, err := io.Copy(targetFile, io.TeeReader(resp.Body, progress)); err != nil || n != int64(d.asset.Size) {
			progress.Err = errors.New("Download error")
			return
		}

		//uncompress, do not return progress until it finishes.
		err = d.uncompressToDir(targetFile, d.destDir)
		if err != nil {
			progress.Err = err
			return
		}

		if onFinished != nil {
			onFinished()
		}
	}()

	return progress
}

func (d *Downloader) uncompressToDir(targetFile *os.File, destDir string) error {
	if !strings.HasSuffix(targetFile.Name(), ".tar.gz") {
		return errors.New("Unknown release format: " + targetFile.Name())
	}

	targetFile.Seek(0, io.SeekStart)

	// Extract through utils so archive entries cannot escape destDir.
	return utils.ExtractTarGzFromReader(targetFile, destDir)
}

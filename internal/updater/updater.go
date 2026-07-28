package updater

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
)

const (
	repoOwner              = "SEObserver"
	repoName               = "crawlobserver"
	maxReleaseResponseSize = 2 << 20
	maxBinarySize          = 256 << 20
	maxDesktopArchiveSize  = 512 << 20
	maxDesktopExpandedSize = 1 << 30
	maxDesktopEntries      = 20_000
)

var updateHTTPClient = &http.Client{
	Timeout: 2 * time.Minute,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refusing update redirect to non-HTTPS URL %q", req.URL)
		}
		if len(via) >= 10 {
			return fmt.Errorf("too many update download redirects")
		}
		return nil
	},
}

// Release represents a GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Assets  []Asset `json:"assets"`
	HTMLURL string  `json:"html_url"`
}

// Asset represents a release asset.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

// Version is the current build version, set at build time via ldflags.
var Version = "dev"

// UpdateStatus represents the current update check state.
type UpdateStatus struct {
	mu             sync.RWMutex
	Available      bool      `json:"available"`
	CurrentVersion string    `json:"current_version"`
	LatestVersion  string    `json:"latest_version"`
	ReleaseURL     string    `json:"release_url"`
	CheckedAt      time.Time `json:"checked_at"`
	Error          string    `json:"error,omitempty"`
	release        *Release
}

// NewUpdateStatus creates a new UpdateStatus.
func NewUpdateStatus() *UpdateStatus {
	return &UpdateStatus{CurrentVersion: Version}
}

// Check performs a background update check and updates the status.
func (s *UpdateStatus) Check() {
	release, available, err := CheckUpdate()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.CheckedAt = time.Now()
	if err != nil {
		s.Error = err.Error()
		return
	}
	s.Error = ""
	s.Available = available
	s.release = release
	if release != nil {
		s.LatestVersion = strings.TrimPrefix(release.TagName, "v")
		s.ReleaseURL = release.HTMLURL
	}
}

// Snapshot returns a copy safe for JSON serialization.
func (s *UpdateStatus) Snapshot() UpdateStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return UpdateStatus{
		Available:      s.Available,
		CurrentVersion: s.CurrentVersion,
		LatestVersion:  s.LatestVersion,
		ReleaseURL:     s.ReleaseURL,
		CheckedAt:      s.CheckedAt,
		Error:          s.Error,
	}
}

// Release returns the cached release (may be nil).
func (s *UpdateStatus) Release() *Release {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.release
}

// CheckUpdate checks if a newer version is available on GitHub.
func CheckUpdate() (*Release, bool, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)

	resp, err := updateHTTPClient.Get(url)
	if err != nil {
		return nil, false, fmt.Errorf("checking for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("checking for updates: GitHub returned %s", resp.Status)
	}

	var release Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseResponseSize)).Decode(&release); err != nil {
		return nil, false, fmt.Errorf("parsing release: %w", err)
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(Version, "v")

	if latest != current && current != "dev" {
		return &release, true, nil
	}

	return &release, false, nil
}

// DownloadUpdate downloads the appropriate binary for the current platform.
func DownloadUpdate(release *Release) (string, error) {
	if release == nil {
		return "", fmt.Errorf("release metadata is missing")
	}
	assetName := expectedAssetName()
	asset := findAsset(release, assetName)
	if asset == nil {
		return "", fmt.Errorf("no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	applog.Infof("updater", "Downloading %s...", assetName)

	tmpPath, err := downloadVerifiedAsset(*asset, "crawlobserver-update-*", maxBinarySize)
	if err != nil {
		return "", err
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("chmod: %w", err)
	}

	return tmpPath, nil
}

// SelfUpdate replaces the current binary with the downloaded update.
func SelfUpdate(newBinaryPath string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	// Rename current binary as backup
	backupPath := execPath + ".bak"
	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	// Move new binary into place
	if err := os.Rename(newBinaryPath, execPath); err != nil {
		// Restore backup on failure
		os.Rename(backupPath, execPath)
		return fmt.Errorf("installing update: %w", err)
	}

	// Remove backup
	os.Remove(backupPath)

	applog.Info("updater", "Update installed. Restart to use the new version.")
	return nil
}

func expectedAssetName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("crawlobserver-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

// ExpectedDesktopAssetName returns the expected .app.tar.gz asset name for desktop updates.
func ExpectedDesktopAssetName() string {
	return "CrawlObserver-macOS.app.tar.gz"
}

// DownloadDesktopUpdate downloads and extracts the .app.tar.gz for desktop mode.
// Returns the path to the extracted .app bundle.
func DownloadDesktopUpdate(release *Release) (string, error) {
	if release == nil {
		return "", fmt.Errorf("release metadata is missing")
	}
	assetName := ExpectedDesktopAssetName()
	asset := findAsset(release, assetName)
	if asset == nil {
		return "", fmt.Errorf("no desktop bundle found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	applog.Infof("updater", "Downloading %s...", assetName)

	archivePath, err := downloadVerifiedAsset(*asset, "crawlobserver-desktop-update-*.tar.gz", maxDesktopArchiveSize)
	if err != nil {
		return "", err
	}
	defer os.Remove(archivePath)

	return extractDesktopArchive(archivePath)
}

func findAsset(release *Release, name string) *Asset {
	if release == nil {
		return nil
	}
	for i := range release.Assets {
		if release.Assets[i].Name == name {
			return &release.Assets[i]
		}
	}
	return nil
}

func downloadVerifiedAsset(asset Asset, pattern string, maxSize int64) (string, error) {
	if asset.Size <= 0 {
		return "", fmt.Errorf("release asset %s has invalid size %d", asset.Name, asset.Size)
	}
	if asset.Size > maxSize {
		return "", fmt.Errorf("release asset %s is too large: %d bytes (maximum %d)", asset.Name, asset.Size, maxSize)
	}

	expectedDigest, err := parseSHA256Digest(asset.Digest)
	if err != nil {
		return "", fmt.Errorf("release asset %s: %w", asset.Name, err)
	}

	downloadURL, err := url.Parse(asset.BrowserDownloadURL)
	if err != nil {
		return "", fmt.Errorf("invalid update URL: %w", err)
	}
	if downloadURL.Scheme != "https" || downloadURL.Host == "" {
		return "", fmt.Errorf("refusing non-HTTPS update URL %q", asset.BrowserDownloadURL)
	}

	req, err := http.NewRequest(http.MethodGet, downloadURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("creating update request: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", asset.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return "", fmt.Errorf("downloading %s: server returned %s: %s", asset.Name, resp.Status, strings.TrimSpace(string(body)))
	}
	if resp.ContentLength > maxSize {
		return "", fmt.Errorf("release asset %s exceeds maximum size %d", asset.Name, maxSize)
	}

	tmpFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	keep := false
	defer func() {
		if !keep {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmpFile, hash), io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return "", fmt.Errorf("writing update: %w", err)
	}
	if written > maxSize {
		return "", fmt.Errorf("release asset %s exceeds maximum size %d", asset.Name, maxSize)
	}
	if written != asset.Size {
		return "", fmt.Errorf("release asset %s size mismatch: got %d bytes, expected %d", asset.Name, written, asset.Size)
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != expectedDigest {
		return "", fmt.Errorf("release asset %s SHA-256 mismatch", asset.Name)
	}
	if err := tmpFile.Sync(); err != nil {
		return "", fmt.Errorf("syncing update: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("closing update: %w", err)
	}

	keep = true
	return tmpPath, nil
}

func parseSHA256Digest(digest string) (string, error) {
	algorithm, encoded, ok := strings.Cut(strings.TrimSpace(digest), ":")
	if !ok || !strings.EqualFold(algorithm, "sha256") {
		return "", fmt.Errorf("missing or unsupported GitHub digest")
	}
	if len(encoded) != sha256.Size*2 {
		return "", fmt.Errorf("invalid SHA-256 digest length")
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return "", fmt.Errorf("invalid SHA-256 digest: %w", err)
	}
	return strings.ToLower(encoded), nil
}

func extractDesktopArchive(archivePath string) (string, error) {
	archive, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening desktop update: %w", err)
	}
	defer archive.Close()

	gr, err := gzip.NewReader(archive)
	if err != nil {
		return "", fmt.Errorf("gzip: %w", err)
	}
	defer gr.Close()

	tmpDir, err := os.MkdirTemp("", "crawlobserver-desktop-update-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(tmpDir)
		}
	}()

	tr := tar.NewReader(gr)
	var appPath string
	var expandedSize int64
	entryCount := 0

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tar: %w", err)
		}

		entryCount++
		if entryCount > maxDesktopEntries {
			return "", fmt.Errorf("desktop update archive contains too many entries")
		}

		cleanName := filepath.Clean(strings.TrimPrefix(hdr.Name, "./"))
		if cleanName == "." || filepath.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("unsafe path %q in desktop update archive", hdr.Name)
		}
		dest := filepath.Join(tmpDir, cleanName)
		rel, err := filepath.Rel(tmpDir, dest)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("unsafe path %q in desktop update archive", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			mode := os.FileMode(hdr.Mode).Perm()
			if mode == 0 {
				mode = 0755
			}
			if err := os.MkdirAll(dest, mode); err != nil {
				return "", fmt.Errorf("creating %s: %w", cleanName, err)
			}
			if !strings.Contains(cleanName, string(filepath.Separator)) && strings.HasSuffix(cleanName, ".app") && appPath == "" {
				appPath = dest
			}
		case tar.TypeReg:
			if hdr.Size < 0 || hdr.Size > maxDesktopExpandedSize-expandedSize {
				return "", fmt.Errorf("desktop update archive exceeds expanded size limit")
			}
			expandedSize += hdr.Size
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return "", fmt.Errorf("creating parent directory for %s: %w", cleanName, err)
			}
			mode := os.FileMode(hdr.Mode).Perm()
			if mode == 0 {
				mode = 0644
			}
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return "", fmt.Errorf("extracting %s: %w", cleanName, err)
			}
			if _, err := io.CopyN(f, tr, hdr.Size); err != nil {
				f.Close()
				return "", fmt.Errorf("extracting %s: %w", cleanName, err)
			}
			if err := f.Close(); err != nil {
				return "", fmt.Errorf("closing %s: %w", cleanName, err)
			}
		default:
			return "", fmt.Errorf("unsupported archive entry %q in desktop update", hdr.Name)
		}

		// Track top-level .app directory
		parts := strings.SplitN(cleanName, string(filepath.Separator), 2)
		if strings.HasSuffix(parts[0], ".app") && appPath == "" {
			appPath = filepath.Join(tmpDir, parts[0])
		}
	}

	if appPath == "" {
		return "", fmt.Errorf("no .app bundle found in archive")
	}
	info, err := os.Stat(appPath)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("invalid .app bundle in archive")
	}

	applog.Infof("updater", "Desktop update extracted to %s", appPath)
	keep = true
	return appPath, nil
}

// SelfUpdateDesktop replaces the current .app bundle with the new one.
func SelfUpdateDesktop(newAppPath string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	// Walk up from the binary to find the .app bundle
	// e.g. /path/to/CrawlObserver.app/Contents/MacOS/CrawlObserver -> /path/to/CrawlObserver.app
	currentApp := execPath
	for !strings.HasSuffix(currentApp, ".app") {
		parent := filepath.Dir(currentApp)
		if parent == currentApp {
			return fmt.Errorf("could not find .app bundle from executable path: %s", execPath)
		}
		currentApp = parent
	}

	backupPath := currentApp + ".bak"

	// Rename current .app as backup
	if err := os.Rename(currentApp, backupPath); err != nil {
		return fmt.Errorf("backing up current app: %w", err)
	}

	// Move new .app into place
	if err := os.Rename(newAppPath, currentApp); err != nil {
		os.Rename(backupPath, currentApp)
		return fmt.Errorf("installing update: %w", err)
	}

	// Remove backup
	os.RemoveAll(backupPath)

	applog.Info("updater", "Desktop update installed. Restart the application to use the new version.")
	return nil
}

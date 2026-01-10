package ffi

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	defaultShimFlavor = "basic"

	envShimFlavor          = "LIBWEBRTC_SHIM_FLAVOR"
	envShimCacheDir        = "LIBWEBRTC_SHIM_CACHE_DIR"
	envShimBaseURL         = "LIBWEBRTC_SHIM_BASE_URL"
	envShimDisableDownload = "LIBWEBRTC_SHIM_DISABLE_DOWNLOAD"
)

const (
	downloadTimeout   = 10 * time.Minute
	downloadLockDelay = 200 * time.Millisecond
	downloadLockTries = 50
)

type shimManifest struct {
	SchemaVersion int                       `json:"schema_version"`
	BaseURL       string                    `json:"base_url"`
	Flavors       map[string]shimFlavorInfo `json:"flavors"`
}

type shimFlavorInfo struct {
	ReleaseTag string                  `json:"release_tag"`
	Assets     map[string]shimAssetRef `json:"assets"`
}

type shimAssetRef struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

//go:embed shim_manifest.json
var shimManifestData []byte

var (
	shimManifestOnce   sync.Once
	shimManifestErr    error
	shimManifestCached *shimManifest
)

func resolveLibrary() (string, error, error) {
	if path, ok := findLocalLibrary(); ok {
		return path, nil, nil
	}

	if isDownloadDisabled() {
		return getLibraryName(), nil, nil
	}

	path, err := downloadShim()
	if err != nil {
		return getLibraryName(), err, nil
	}

	return path, nil, nil
}

func isDownloadDisabled() bool {
	value := strings.TrimSpace(os.Getenv(envShimDisableDownload))
	if value == "" {
		return false
	}
	value = strings.ToLower(value)
	return value != "0" && value != "false"
}

func downloadShim() (string, error) {
	manifest, err := loadShimManifest()
	if err != nil {
		return "", err
	}

	flavor := shimFlavor()
	flavorInfo, ok := manifest.Flavors[flavor]
	if !ok {
		return "", fmt.Errorf("shim manifest missing flavor %q", flavor)
	}

	platform, err := shimPlatformKey()
	if err != nil {
		return "", err
	}

	asset, ok := flavorInfo.Assets[platform]
	if !ok {
		return "", fmt.Errorf("shim manifest missing asset for %s flavor %q", platform, flavor)
	}
	if asset.File == "" {
		return "", fmt.Errorf("shim manifest missing file for %s flavor %q", platform, flavor)
	}
	if !isValidSHA256(asset.SHA256) {
		return "", fmt.Errorf("shim manifest has invalid sha256 for %s flavor %q: %q", platform, flavor, asset.SHA256)
	}

	baseURL := strings.TrimRight(manifest.BaseURL, "/")
	if override := strings.TrimSpace(os.Getenv(envShimBaseURL)); override != "" {
		baseURL = strings.TrimRight(override, "/")
	}
	if baseURL == "" {
		return "", errors.New("shim manifest base_url is empty")
	}

	if flavorInfo.ReleaseTag == "" {
		return "", fmt.Errorf("shim manifest missing release_tag for flavor %q", flavor)
	}

	url := fmt.Sprintf("%s/%s/%s", baseURL, flavorInfo.ReleaseTag, asset.File)

	cacheRoot, err := shimCacheRoot()
	if err != nil {
		return "", err
	}

	libName := getLibraryName()
	destDir := filepath.Join(cacheRoot, "shim", flavor, flavorInfo.ReleaseTag, platform)
	libPath := filepath.Join(destDir, libName)

	if _, err := os.Stat(libPath); err == nil {
		return libPath, nil
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create shim cache dir: %w", err)
	}

	if err := withDownloadLock(destDir, func() error {
		if _, err := os.Stat(libPath); err == nil {
			return nil
		}
		return downloadAndInstallShim(url, asset.SHA256, destDir, libName)
	}); err != nil {
		return "", err
	}

	if _, err := os.Stat(libPath); err != nil {
		return "", fmt.Errorf("shim not found after download: %s", libPath)
	}

	return libPath, nil
}

func shimFlavor() string {
	if flavor := strings.TrimSpace(os.Getenv(envShimFlavor)); flavor != "" {
		return strings.ToLower(flavor)
	}
	return defaultShimFlavor
}

func shimPlatformKey() (string, error) {
	return shimPlatformKeyFor(runtime.GOOS, runtime.GOARCH)
}

func shimPlatformKeyFor(goos, goarch string) (string, error) {
	switch goos {
	case "darwin":
		switch goarch {
		case "arm64":
			return "darwin_arm64", nil
		case "amd64":
			return "darwin_amd64", nil
		}
	case "linux":
		switch goarch {
		case "arm64":
			return "linux_arm64", nil
		case "amd64":
			return "linux_amd64", nil
		}
	}
	return "", fmt.Errorf("unsupported platform for auto-download: %s/%s", goos, goarch)
}

func shimCacheRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv(envShimCacheDir)); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".libgowebrtc"), nil
}

func loadShimManifest() (*shimManifest, error) {
	shimManifestOnce.Do(func() {
		var parsed shimManifest
		if err := json.Unmarshal(shimManifestData, &parsed); err != nil {
			shimManifestErr = fmt.Errorf("parse shim manifest: %w", err)
			return
		}
		shimManifestCached = &parsed
	})

	if shimManifestErr != nil {
		return nil, shimManifestErr
	}
	if shimManifestCached == nil {
		return nil, errors.New("shim manifest not loaded")
	}
	return shimManifestCached, nil
}

func withDownloadLock(dir string, fn func() error) error {
	lockPath := filepath.Join(dir, ".download.lock")

	for i := 0; i < downloadLockTries; i++ {
		lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = lockFile.Close()
			defer os.Remove(lockPath)
			return fn()
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create download lock: %w", err)
		}
		time.Sleep(downloadLockDelay)
	}

	return fmt.Errorf("timeout waiting for shim download lock in %s", dir)
}

func downloadAndInstallShim(url, expectedSHA256, destDir, libName string) error {
	tmpFile, err := os.CreateTemp(destDir, "shim-download-*.tgz")
	if err != nil {
		return fmt.Errorf("create download temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download shim archive: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download shim archive: unexpected status %s", resp.Status)
	}

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmpFile, hasher), resp.Body); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("download shim archive: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("finalize shim archive: %w", err)
	}

	actualSHA := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualSHA, expectedSHA256) {
		return fmt.Errorf("shim archive sha256 mismatch: expected %s, got %s", expectedSHA256, actualSHA)
	}

	extractDir, err := os.MkdirTemp(destDir, "shim-extract-")
	if err != nil {
		return fmt.Errorf("create extract dir: %w", err)
	}
	defer os.RemoveAll(extractDir)

	if err := extractTarGz(tmpPath, extractDir); err != nil {
		return fmt.Errorf("extract shim archive: %w", err)
	}

	foundLib, err := findFileByName(extractDir, libName)
	if err != nil {
		return err
	}

	finalPath := filepath.Join(destDir, libName)
	if err := moveFile(foundLib, finalPath); err != nil {
		return fmt.Errorf("install shim: %w", err)
	}

	if runtime.GOOS != "windows" {
		_ = os.Chmod(finalPath, 0o755)
	}

	copyOptionalFile(extractDir, "LICENSE", filepath.Join(destDir, "LICENSE"))
	copyOptionalFile(extractDir, "NOTICE", filepath.Join(destDir, "NOTICE"))

	return nil
}

func extractTarGz(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		cleanName := filepath.Clean(header.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			return fmt.Errorf("invalid archive path: %s", header.Name)
		}

		targetPath := filepath.Join(destDir, cleanName)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tarReader); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry: %s", header.Name)
		}
	}
}

func findFileByName(rootDir, name string) (string, error) {
	var found string
	err := filepath.WalkDir(rootDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Name() == name {
			found = path
			return io.EOF
		}
		return nil
	})
	if errors.Is(err, io.EOF) && found != "" {
		return found, nil
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("file %s not found in archive", name)
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func copyOptionalFile(rootDir, name, destPath string) {
	if existing, err := findFileByName(rootDir, name); err == nil {
		_ = copyFile(existing, destPath)
	}
}

func isValidSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

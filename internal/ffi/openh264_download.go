package ffi

import (
	"compress/bzip2"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
)

const (
	defaultOpenH264Version   = "2.5.1"
	defaultOpenH264SOVersion = "7"
	defaultOpenH264BaseURL   = "https://ciscobinary.openh264.org"

	envOpenH264Path            = "LIBWEBRTC_OPENH264_PATH"
	envOpenH264URL             = "LIBWEBRTC_OPENH264_URL"
	envOpenH264Version         = "LIBWEBRTC_OPENH264_VERSION"
	envOpenH264BaseURL         = "LIBWEBRTC_OPENH264_BASE_URL"
	envOpenH264SOVersion       = "LIBWEBRTC_OPENH264_SOVERSION"
	envOpenH264CacheDir        = "LIBWEBRTC_OPENH264_CACHE_DIR"
	envOpenH264DisableDownload = "LIBWEBRTC_OPENH264_DISABLE_DOWNLOAD"
	envOpenH264SHA256          = "LIBWEBRTC_OPENH264_SHA256"
	envPreferSoftwareCodecs    = "LIBWEBRTC_PREFER_SOFTWARE_CODECS"
)

var (
	openh264Once           sync.Once
	openh264Err            error
	openh264Path           string
	openh264Handle         uintptr
	errOpenH264Unsupported = errors.New("openh264 binary not published for this platform")
)

func ensureOpenH264(required bool) error {
	if !required {
		return nil
	}

	openh264Once.Do(func() {
		path, err := resolveOpenH264()
		if err != nil {
			openh264Err = err
			return
		}
		if path == "" {
			openh264Err = errors.New("openh264 not available; set LIBWEBRTC_OPENH264_PATH or enable download")
			return
		}

		addLibraryDirToEnv(filepath.Dir(path))

		handle, err := dlopenLibrary(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			openh264Err = fmt.Errorf("load openh264: %w", err)
			return
		}

		openh264Handle = handle
		openh264Path = path
	})

	return openh264Err
}

func shouldPreferSoftwareCodecs() bool {
	value := strings.TrimSpace(os.Getenv(envPreferSoftwareCodecs))
	if value == "" {
		return false
	}
	value = strings.ToLower(value)
	return value != "0" && value != "false"
}

func shouldRequireOpenH264(preferHW bool) bool {
	if shouldPreferSoftwareCodecs() {
		return true
	}
	return !preferHW
}

func resolveOpenH264() (string, error) {
	if path := strings.TrimSpace(os.Getenv(envOpenH264Path)); path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("openh264 path not found: %w", err)
		}
		return path, nil
	}

	if isOpenH264DownloadDisabled() {
		return "", nil
	}

	return downloadOpenH264()
}

func isOpenH264DownloadDisabled() bool {
	value := strings.TrimSpace(os.Getenv(envOpenH264DisableDownload))
	if value == "" {
		return false
	}
	value = strings.ToLower(value)
	return value != "0" && value != "false"
}

type openh264Spec struct {
	URL       string
	Version   string
	Platform  string
	LibName   string
	Archive   string
	SHA256    string
	CacheRoot string
}

func downloadOpenH264() (string, error) {
	spec, err := openh264DownloadSpec()
	if err != nil {
		return "", err
	}

	destDir := filepath.Join(spec.CacheRoot, "openh264", spec.Version, spec.Platform)
	libPath := filepath.Join(destDir, spec.LibName)

	if _, err := os.Stat(libPath); err == nil {
		return libPath, nil
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create openh264 cache dir: %w", err)
	}

	if err := withDownloadLock(destDir, func() error {
		if _, err := os.Stat(libPath); err == nil {
			return nil
		}
		return downloadAndInstallOpenH264(spec, destDir)
	}); err != nil {
		return "", err
	}

	if _, err := os.Stat(libPath); err != nil {
		return "", fmt.Errorf("openh264 not found after download: %s", libPath)
	}

	return libPath, nil
}

func openh264DownloadSpec() (openh264Spec, error) {
	version := strings.TrimSpace(os.Getenv(envOpenH264Version))
	urlOverride := strings.TrimSpace(os.Getenv(envOpenH264URL))
	if version == "" {
		if urlOverride != "" {
			version = "custom"
		} else {
			version = defaultOpenH264Version
		}
	}

	platform, err := openh264PlatformKey()
	if err != nil {
		return openh264Spec{}, err
	}

	libName, err := openh264LibraryName()
	if err != nil {
		return openh264Spec{}, err
	}

	archive := ""
	url := urlOverride
	if url == "" {
		archive, err = openh264ArchiveName(version)
		if err != nil {
			return openh264Spec{}, err
		}
		baseURL := strings.TrimRight(defaultOpenH264BaseURL, "/")
		if override := strings.TrimSpace(os.Getenv(envOpenH264BaseURL)); override != "" {
			baseURL = strings.TrimRight(override, "/")
		}
		url = fmt.Sprintf("%s/%s", baseURL, archive)
	}

	cacheRoot, err := openh264CacheRoot()
	if err != nil {
		return openh264Spec{}, err
	}

	sha := strings.TrimSpace(os.Getenv(envOpenH264SHA256))
	if sha != "" && !isValidSHA256(sha) {
		return openh264Spec{}, fmt.Errorf("invalid openh264 sha256: %q", sha)
	}

	return openh264Spec{
		URL:       url,
		Version:   version,
		Platform:  platform,
		LibName:   libName,
		Archive:   archive,
		SHA256:    sha,
		CacheRoot: cacheRoot,
	}, nil
}

func openh264PlatformKey() (string, error) {
	return openh264PlatformKeyFor(runtime.GOOS, runtime.GOARCH)
}

func openh264PlatformKeyFor(goos, goarch string) (string, error) {
	switch goos {
	case "darwin":
		switch goarch {
		case "amd64":
			return "darwin_amd64", nil
		case "arm64":
			return "darwin_arm64", nil
		}
	case "linux":
		switch goarch {
		case "amd64":
			return "linux_amd64", nil
		case "386":
			return "linux_386", nil
		case "arm":
			return "linux_arm", nil
		case "arm64":
			return "linux_arm64", nil
		}
	case "windows":
		switch goarch {
		case "amd64":
			return "windows_amd64", nil
		case "386":
			return "windows_386", nil
		case "arm64":
			return "windows_arm64", nil
		}
	}
	return "", fmt.Errorf("unsupported platform for openh264: %s/%s", goos, goarch)
}

func openh264ArchiveName(version string) (string, error) {
	return openh264ArchiveNameFor(runtime.GOOS, runtime.GOARCH, version, openh264SOVersion())
}

func openh264ArchiveNameFor(goos, goarch, version, soVersion string) (string, error) {
	switch goos {
	case "darwin":
		switch goarch {
		case "amd64":
			return fmt.Sprintf("libopenh264-%s-mac-x64.dylib.bz2", version), nil
		case "arm64":
			return fmt.Sprintf("libopenh264-%s-mac-arm64.dylib.bz2", version), nil
		}
	case "linux":
		switch goarch {
		case "amd64":
			return fmt.Sprintf("libopenh264-%s-linux64.%s.so.bz2", version, soVersion), nil
		case "386":
			return fmt.Sprintf("libopenh264-%s-linux32.%s.so.bz2", version, soVersion), nil
		case "arm":
			return fmt.Sprintf("libopenh264-%s-linux-arm.%s.so.bz2", version, soVersion), nil
		case "arm64":
			return fmt.Sprintf("libopenh264-%s-linux-arm64.%s.so.bz2", version, soVersion), nil
		}
	case "windows":
		switch goarch {
		case "amd64":
			return fmt.Sprintf("openh264-%s-win64.dll.bz2", version), nil
		case "386":
			return fmt.Sprintf("openh264-%s-win32.dll.bz2", version), nil
		case "arm64":
			return fmt.Sprintf("openh264-%s-win-arm64.dll.bz2", version), nil
		}
	}
	return "", fmt.Errorf("%w: %s/%s", errOpenH264Unsupported, goos, goarch)
}

func openh264LibraryName() (string, error) {
	return openh264LibraryNameFor(runtime.GOOS, openh264SOVersion())
}

func openh264LibraryNameFor(goos, soVersion string) (string, error) {
	switch goos {
	case "darwin":
		return "libopenh264.dylib", nil
	case "linux":
		return fmt.Sprintf("libopenh264.so.%s", soVersion), nil
	case "windows":
		return "openh264.dll", nil
	}
	return "", fmt.Errorf("no openh264 library name for %s", goos)
}

func openh264SOVersion() string {
	if override := strings.TrimSpace(os.Getenv(envOpenH264SOVersion)); override != "" {
		return override
	}
	return defaultOpenH264SOVersion
}

func openh264CacheRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv(envOpenH264CacheDir)); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".libgowebrtc"), nil
}

func downloadAndInstallOpenH264(spec openh264Spec, destDir string) error {
	tmpFile, err := os.CreateTemp(destDir, "openh264-download-*.bz2")
	if err != nil {
		return fmt.Errorf("create openh264 temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(spec.URL)
	if err != nil {
		return fmt.Errorf("download openh264 archive: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download openh264 archive: unexpected status %s", resp.Status)
	}

	var hasher hashWriter
	writer := io.Writer(tmpFile)
	if spec.SHA256 != "" {
		hasher = newHashWriter()
		writer = io.MultiWriter(tmpFile, hasher)
	}

	if _, copyErr := io.Copy(writer, resp.Body); copyErr != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("download openh264 archive: %w", copyErr)
	}
	if closeErr := tmpFile.Close(); closeErr != nil {
		return fmt.Errorf("finalize openh264 archive: %w", closeErr)
	}

	if spec.SHA256 != "" {
		actualSHA := hasher.Sum()
		if !strings.EqualFold(actualSHA, spec.SHA256) {
			return fmt.Errorf("openh264 archive sha256 mismatch: expected %s, got %s", spec.SHA256, actualSHA)
		}
	}

	tmpOut, err := os.CreateTemp(destDir, "openh264-extract-*")
	if err != nil {
		return fmt.Errorf("create openh264 extract temp: %w", err)
	}
	tmpOutPath := tmpOut.Name()
	_ = tmpOut.Close()
	defer os.Remove(tmpOutPath)

	if strings.HasSuffix(strings.ToLower(spec.URL), ".bz2") {
		if err := extractBzip2(tmpPath, tmpOutPath); err != nil {
			return fmt.Errorf("extract openh264 archive: %w", err)
		}
	} else if err := copyFile(tmpPath, tmpOutPath); err != nil {
		return fmt.Errorf("copy openh264 archive: %w", err)
	}

	finalPath := filepath.Join(destDir, spec.LibName)
	if err := moveFile(tmpOutPath, finalPath); err != nil {
		return fmt.Errorf("install openh264: %w", err)
	}

	if runtime.GOOS != "windows" {
		_ = os.Chmod(finalPath, 0o755)
	}

	if runtime.GOOS == "linux" {
		linkPath := filepath.Join(destDir, "libopenh264.so")
		_ = os.Remove(linkPath)
		_ = os.Symlink(spec.LibName, linkPath)
	}

	return nil
}

func extractBzip2(srcPath, destPath string) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()

	reader := bzip2.NewReader(in)
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.CopyN(out, reader, 100<<20); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return out.Sync()
}

func addLibraryDirToEnv(dir string) {
	if dir == "" {
		return
	}

	var envVar string
	switch runtime.GOOS {
	case "windows":
		envVar = "PATH"
	case "darwin":
		envVar = "DYLD_LIBRARY_PATH"
	default:
		envVar = "LD_LIBRARY_PATH"
	}

	existing := os.Getenv(envVar)
	if pathListHasDir(existing, dir) {
		return
	}

	if existing == "" {
		_ = os.Setenv(envVar, dir)
		return
	}

	_ = os.Setenv(envVar, dir+string(filepath.ListSeparator)+existing)
}

func pathListHasDir(list, dir string) bool {
	for _, entry := range filepath.SplitList(list) {
		if entry == dir {
			return true
		}
	}
	return false
}

type hashWriter struct {
	hash hash.Hash
}

func newHashWriter() hashWriter {
	return hashWriter{hash: sha256.New()}
}

func (w hashWriter) Write(p []byte) (int, error) {
	return w.hash.Write(p)
}

func (w hashWriter) Sum() string {
	return hex.EncodeToString(w.hash.Sum(nil))
}

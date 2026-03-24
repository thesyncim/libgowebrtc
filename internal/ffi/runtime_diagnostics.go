package ffi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DiagnosticStatus describes the outcome of a runtime diagnostic check.
type DiagnosticStatus string

const (
	DiagnosticStatusUnknown    DiagnosticStatus = "unknown"
	DiagnosticStatusOK         DiagnosticStatus = "ok"
	DiagnosticStatusMissing    DiagnosticStatus = "missing"
	DiagnosticStatusMismatch   DiagnosticStatus = "mismatch"
	DiagnosticStatusUnverified DiagnosticStatus = "unverified"
)

// DependencyReport summarizes the runtime state for a dependency used by libgowebrtc.
type DependencyReport struct {
	Name                     string
	Path                     string
	Source                   string
	CacheDir                 string
	Available                bool
	DownloadDisabled         bool
	ExpectedVersion          string
	ActualVersion            string
	ExpectedLibWebRTCVersion string
	ActualLibWebRTCVersion   string
	VersionStatus            DiagnosticStatus
	ExpectedSHA256           string
	ActualSHA256             string
	ChecksumStatus           DiagnosticStatus
	BlockingIssues           []string
	Warnings                 []string
}

// RuntimeDiagnosticsReport summarizes libgowebrtc runtime dependency state.
type RuntimeDiagnosticsReport struct {
	Ready    bool
	Shim     DependencyReport
	OpenH264 DependencyReport
	Warnings []string
}

// CheckRuntimeDependencies reports the currently configured runtime dependency state
// without downloading artifacts or mutating process-global loader state.
func CheckRuntimeDependencies() (*RuntimeDiagnosticsReport, error) {
	shimReport, err := inspectShimDependency()
	if err != nil {
		return nil, err
	}

	openh264Report, err := inspectOpenH264Dependency()
	if err != nil {
		return nil, err
	}

	report := &RuntimeDiagnosticsReport{
		Shim:     shimReport,
		OpenH264: openh264Report,
	}
	report.Ready = len(report.Shim.BlockingIssues) == 0 && len(report.OpenH264.BlockingIssues) == 0
	if !report.Ready {
		report.Warnings = append(report.Warnings, "one or more runtime dependency checks reported blocking issues")
	}

	return report, nil
}

func inspectShimDependency() (DependencyReport, error) {
	report := DependencyReport{
		Name:                     "libwebrtc_shim",
		DownloadDisabled:         isDownloadDisabled(),
		ExpectedVersion:          ExpectedShimVersion,
		ExpectedLibWebRTCVersion: ExpectedLibWebRTCVersion,
		VersionStatus:            DiagnosticStatusUnknown,
		ChecksumStatus:           DiagnosticStatusUnknown,
	}

	cacheRoot, err := shimCacheRoot()
	if err != nil {
		return report, err
	}
	report.CacheDir = filepath.Join(cacheRoot, "shim")

	var (
		manifest       *shimManifest
		manifestErr    error
		platform       string
		platformErr    error
		downloadedPath string
	)
	manifest, manifestErr = loadShimManifest()
	platform, platformErr = shimPlatformKey()
	if manifestErr == nil && platformErr == nil {
		flavor := shimFlavor()
		if flavorInfo, ok := manifest.Flavors[flavor]; ok {
			if asset, ok := flavorInfo.Assets[platform]; ok {
				if asset.SHA256 != "" {
					report.ExpectedSHA256 = strings.ToLower(asset.SHA256)
				}
				if flavorInfo.ReleaseTag != "" {
					downloadedPath = filepath.Join(cacheRoot, "shim", flavor, flavorInfo.ReleaseTag, platform, getLibraryName())
				}
			}
		}
	}

	localPath, localFound, localErr := findLocalLibrary()
	switch {
	case localErr != nil:
		report.Path = localPath
		report.Source = diagnosticSource(localPath, cacheRoot)
		if report.Source != "downloaded" {
			report.ExpectedSHA256 = ""
		}
		report.BlockingIssues = append(report.BlockingIssues, localErr.Error())
	case localFound:
		report.Path = localPath
		report.Source = diagnosticSource(localPath, cacheRoot)
		if report.Source != "downloaded" {
			report.ExpectedSHA256 = ""
		}
	default:
		switch {
		case platformErr != nil:
			report.BlockingIssues = append(report.BlockingIssues, platformErr.Error())
		case manifestErr != nil:
			report.BlockingIssues = append(report.BlockingIssues, fmt.Sprintf("load shim manifest: %v", manifestErr))
		default:
			flavor := shimFlavor()
			flavorInfo, ok := manifest.Flavors[flavor]
			if !ok {
				report.BlockingIssues = append(report.BlockingIssues, fmt.Sprintf("shim manifest missing flavor %q", flavor))
				break
			}
			if _, ok := flavorInfo.Assets[platform]; !ok {
				report.BlockingIssues = append(report.BlockingIssues, fmt.Sprintf("shim manifest missing asset for %s flavor %q", platform, flavor))
				break
			}
			if flavorInfo.ReleaseTag == "" {
				report.BlockingIssues = append(report.BlockingIssues, fmt.Sprintf("shim manifest missing release_tag for flavor %q", flavor))
				break
			}
			report.Path = downloadedPath
			report.Source = "downloaded"
			if report.DownloadDisabled {
				report.Path = ""
				report.Source = ""
				report.BlockingIssues = append(report.BlockingIssues, "shim auto-download is disabled and no local shim was found; set LIBWEBRTC_SHIM_PATH or enable download")
			} else {
				report.Warnings = append(report.Warnings, "shim is not cached locally yet; it will download on first successful load")
			}
		}
	}

	populateFileState(&report)
	populateLoadedShimVersionState(&report)

	return report, nil
}

func inspectOpenH264Dependency() (DependencyReport, error) {
	report := DependencyReport{
		Name:             "OpenH264",
		DownloadDisabled: isOpenH264DownloadDisabled(),
		VersionStatus:    DiagnosticStatusUnknown,
		ChecksumStatus:   DiagnosticStatusUnknown,
	}

	cacheRoot, err := openh264CacheRoot()
	if err != nil {
		return report, err
	}
	report.CacheDir = filepath.Join(cacheRoot, "openh264")

	explicitPath := strings.TrimSpace(os.Getenv(envOpenH264Path))
	if explicitPath != "" {
		report.Path = explicitPath
		report.Source = diagnosticSource(explicitPath, cacheRoot)
	} else {
		spec, specErr := openh264DownloadSpec()
		switch {
		case specErr == nil:
			report.ExpectedVersion = spec.Version
			report.ExpectedSHA256 = strings.ToLower(spec.SHA256)
			report.Path = filepath.Join(spec.CacheRoot, "openh264", spec.Version, spec.Platform, spec.LibName)
			report.Source = "downloaded"
			if report.DownloadDisabled {
				report.Path = ""
				report.Source = ""
				report.Warnings = append(report.Warnings, "openh264 auto-download is disabled; H.264 software codecs require LIBWEBRTC_OPENH264_PATH or an enabled download path")
			} else {
				report.Warnings = append(report.Warnings, "openh264 is not cached locally yet; it will download on first H.264 use")
			}
		case strings.TrimSpace(os.Getenv(envOpenH264URL)) != "":
			report.BlockingIssues = append(report.BlockingIssues, fmt.Sprintf("invalid OpenH264 download configuration: %v", specErr))
		default:
			report.Warnings = append(report.Warnings, fmt.Sprintf("OpenH264 diagnostic is unavailable for this platform/configuration: %v", specErr))
		}
	}

	populateFileState(&report)

	if explicitPath != "" && !report.Available {
		report.BlockingIssues = append(report.BlockingIssues, fmt.Sprintf("LIBWEBRTC_OPENH264_PATH points to a missing file: %s", explicitPath))
	}

	if report.ExpectedSHA256 == "" && report.Available {
		report.ChecksumStatus = DiagnosticStatusUnverified
	}

	return report, nil
}

func populateFileState(report *DependencyReport) {
	if report.Path == "" {
		if report.ChecksumStatus == DiagnosticStatusUnknown {
			report.ChecksumStatus = DiagnosticStatusMissing
		}
		if report.VersionStatus == DiagnosticStatusUnknown && report.ExpectedVersion != "" {
			report.VersionStatus = DiagnosticStatusMissing
		}
		return
	}

	info, err := os.Stat(report.Path)
	if err != nil || info.IsDir() {
		report.Available = false
		report.ChecksumStatus = DiagnosticStatusMissing
		if report.ExpectedVersion != "" {
			report.VersionStatus = DiagnosticStatusMissing
		}
		return
	}

	report.Available = true

	actualSHA, err := sha256File(report.Path)
	if err != nil {
		report.BlockingIssues = append(report.BlockingIssues, fmt.Sprintf("compute checksum for %s: %v", report.Path, err))
		return
	}
	report.ActualSHA256 = actualSHA

	switch {
	case report.ExpectedSHA256 == "":
		report.ChecksumStatus = DiagnosticStatusUnverified
	case strings.EqualFold(report.ExpectedSHA256, actualSHA):
		report.ChecksumStatus = DiagnosticStatusOK
	default:
		report.ChecksumStatus = DiagnosticStatusMismatch
		report.BlockingIssues = append(report.BlockingIssues, fmt.Sprintf("%s checksum mismatch: expected %s, got %s", report.Name, report.ExpectedSHA256, actualSHA))
	}
}

func populateLoadedShimVersionState(report *DependencyReport) {
	if !IsLoaded() {
		if report.Available {
			report.Warnings = append(report.Warnings, "shim version was not inspected because the library is not loaded")
		}
		if report.ExpectedVersion != "" && report.VersionStatus == DiagnosticStatusUnknown {
			report.VersionStatus = DiagnosticStatusUnverified
		}
		return
	}

	report.ActualVersion = ShimVersion()
	report.ActualLibWebRTCVersion = LibWebRTCVersion()
	if report.ActualVersion == "" && report.ActualLibWebRTCVersion == "" {
		report.VersionStatus = DiagnosticStatusMissing
		report.BlockingIssues = append(report.BlockingIssues, "loaded shim did not expose version metadata")
		return
	}

	if report.ActualVersion == report.ExpectedVersion && report.ActualLibWebRTCVersion == report.ExpectedLibWebRTCVersion {
		report.VersionStatus = DiagnosticStatusOK
		return
	}

	report.VersionStatus = DiagnosticStatusMismatch
	if report.ActualVersion != report.ExpectedVersion {
		report.BlockingIssues = append(report.BlockingIssues, fmt.Sprintf("shim version mismatch: expected %s, got %s", report.ExpectedVersion, report.ActualVersion))
	}
	if report.ActualLibWebRTCVersion != report.ExpectedLibWebRTCVersion {
		report.BlockingIssues = append(report.BlockingIssues, fmt.Sprintf("libwebrtc version mismatch: expected %s, got %s", report.ExpectedLibWebRTCVersion, report.ActualLibWebRTCVersion))
	}
}

func diagnosticSource(path, cacheRoot string) string {
	if path == "" {
		return ""
	}
	if isPathWithin(path, cacheRoot) {
		return "downloaded"
	}
	return "local"
}

func isPathWithin(path, root string) bool {
	if path == "" || root == "" {
		return false
	}
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	if cleanPath == cleanRoot {
		return true
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	return err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

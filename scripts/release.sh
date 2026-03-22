#!/bin/bash
#
# Release script for libgowebrtc shim
#
# Publishes a prebuilt shim release directory to GitHub.
#
# Usage:
#   ./scripts/release.sh              # Interactive: prompts for version
#   ./scripts/release.sh 0.4.0        # Publish release/shim-v0.4.0
#   ./scripts/release.sh --dry-run    # Show what would happen
#
# Before publishing, prepare a release directory that contains tarballs and
# matching .sha256 files. After the release finishes, backfill the embedded
# manifest checksums with:
#   python3 scripts/update_shim_manifest_checksums.py --release-dir /path/to/release

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors
log_info()    { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error()   { echo -e "\033[0;31m[ERROR]\033[0m $1"; }
log_warn()    { echo -e "\033[0;33m[WARN]\033[0m $1"; }

show_help() {
    cat << EOF
Shim Release Script
===================

Uploads a prepared local release directory to GitHub and creates the tag.

Usage: ./scripts/release.sh [OPTIONS] [VERSION]

Arguments:
  VERSION     Version number (e.g., 0.4.0). Will create tag shim-v0.4.0

Options:
  --release-dir DIR  Directory containing .tar.gz and .sha256 files
  --target REF       Commit-ish for the release tag (default: HEAD)
  --dry-run          Show what would happen without making changes
  --help             Show this help

Examples:
  ./scripts/release.sh              # Interactive mode
  ./scripts/release.sh 0.4.0        # Publish release/shim-v0.4.0
  ./scripts/release.sh --dry-run    # Preview release

Platforms expected in the local release directory:
  - darwin_arm64  (macOS Apple Silicon)
  - darwin_amd64  (macOS Intel, cross-compiled)
  - linux_386     (Linux x86 32-bit, Docker-built)
  - linux_amd64   (Linux x86_64)
  - windows_amd64 (Windows x64)

Additional validated target:
  - linux_arm     (Linux ARM 32-bit, validated via Docker/source build)
EOF
    exit 0
}

get_latest_shim_tag() {
    git tag -l 'shim-v*' | sort -V | tail -1
}

suggest_next_version() {
    local latest=$(get_latest_shim_tag)
    if [[ -z "$latest" ]]; then
        echo "0.1.0"
        return
    fi

    # Extract version numbers
    local version="${latest#shim-v}"
    local major minor patch
    IFS='.' read -r major minor patch <<< "$version"

    # Increment patch
    patch=$((patch + 1))
    echo "${major}.${minor}.${patch}"
}

main() {
    local dry_run=false
    local version=""
    local release_dir=""
    local target_ref="HEAD"

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --release-dir) release_dir="$2"; shift 2 ;;
            --target)      target_ref="$2"; shift 2 ;;
            --dry-run)     dry_run=true; shift ;;
            --help)        show_help ;;
            *)             version="$1"; shift ;;
        esac
    done

    cd "$PROJECT_ROOT"

    # Check git status
    if [[ -n "$(git status --porcelain)" ]]; then
        log_warn "Working directory has uncommitted changes"
        if [[ "$dry_run" == false ]]; then
            read -p "Continue anyway? [y/N] " -n 1 -r
            echo
            [[ ! $REPLY =~ ^[Yy]$ ]] && exit 1
        fi
    fi

    # Get version
    local latest=$(get_latest_shim_tag)
    local suggested=$(suggest_next_version)

    log_info "Latest shim release: ${latest:-none}"

    if [[ -z "$version" ]]; then
        read -p "Enter version [$suggested]: " version
        version="${version:-$suggested}"
    fi

    # Validate version format
    if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        log_error "Invalid version format. Use semantic versioning (e.g., 0.4.0)"
        exit 1
    fi

    local tag="shim-v${version}"

    if [[ -z "$release_dir" ]]; then
        release_dir="$PROJECT_ROOT/release/$tag"
    fi

    if [[ ! -d "$release_dir" ]]; then
        log_error "Release directory not found: $release_dir"
        exit 1
    fi

    if ! find "$release_dir" -maxdepth 1 -name '*.tar.gz' | grep -q .; then
        log_error "No release archives found in $release_dir"
        exit 1
    fi

    if ! find "$release_dir" -maxdepth 1 -name '*.sha256' | grep -q .; then
        log_error "No checksum files found in $release_dir"
        exit 1
    fi

    echo ""
    log_info "Release summary:"
    echo "  Tag:       $tag"
    echo "  Source:    $release_dir"
    echo "  Platforms: darwin_arm64, darwin_amd64, linux_386, linux_amd64, windows_amd64"
    echo "  Extra validated target: linux_arm"
    echo "  Branch:    $(git branch --show-current)"
    echo "  Target:    $target_ref"
    echo "  Commit:    $(git rev-parse --short "$target_ref")"
    echo ""

    if [[ "$dry_run" == true ]]; then
        log_warn "Dry run - no changes made"
        echo "Would run:"
        echo "  gh release create $tag \"$release_dir\"/* --target $target_ref --title '$tag' --notes 'Prebuilt libwebrtc shim assets.'"
        exit 0
    fi

    if gh release view "$tag" >/dev/null 2>&1; then
        log_error "GitHub release $tag already exists"
        exit 1
    fi

    read -p "Create GitHub release $tag from $release_dir? [y/N] " -n 1 -r
    echo
    [[ ! $REPLY =~ ^[Yy]$ ]] && exit 1

    log_info "Creating GitHub release $tag..."
    gh release create \
        "$tag" \
        "$release_dir"/* \
        --target "$target_ref" \
        --title "$tag" \
        --notes "Prebuilt libwebrtc shim assets for darwin_arm64, darwin_amd64, linux_386, linux_amd64, and windows_amd64."

    log_success "GitHub release $tag created!"
    echo ""
    echo "CI will now validate the published assets. Monitor progress at:"
    echo "  https://github.com/thesyncim/libgowebrtc/actions"
    echo ""
    echo "Once complete, the release will be available at:"
    echo "  https://github.com/thesyncim/libgowebrtc/releases/tag/$tag"
}

main "$@"

#!/bin/bash
#
# Release script for libgowebrtc shim
#
# Creates a new shim release by tagging and pushing to GitHub.
# The CI will build all platforms and create the release.
#
# Usage:
#   ./scripts/release.sh              # Interactive: prompts for version
#   ./scripts/release.sh 0.4.0        # Release shim-v0.4.0
#   ./scripts/release.sh --dry-run    # Show what would happen

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

Creates a new shim release by tagging and pushing to GitHub.
CI will automatically build all platforms and create the release.

Usage: ./scripts/release.sh [OPTIONS] [VERSION]

Arguments:
  VERSION     Version number (e.g., 0.4.0). Will create tag shim-v0.4.0

Options:
  --dry-run   Show what would happen without making changes
  --help      Show this help

Examples:
  ./scripts/release.sh              # Interactive mode
  ./scripts/release.sh 0.4.0        # Release shim-v0.4.0
  ./scripts/release.sh --dry-run    # Preview release

Platforms built by CI:
  - darwin_arm64  (macOS Apple Silicon)
  - darwin_amd64  (macOS Intel, cross-compiled)
  - linux_amd64   (Linux x86_64)
  - linux_arm64   (Linux ARM64)
  - windows_amd64 (Windows x64)
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

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dry-run) dry_run=true; shift ;;
            --help)    show_help ;;
            *)         version="$1"; shift ;;
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

    # Check if tag exists
    if git rev-parse "$tag" &>/dev/null; then
        log_error "Tag $tag already exists"
        exit 1
    fi

    echo ""
    log_info "Release summary:"
    echo "  Tag:       $tag"
    echo "  Platforms: darwin_arm64, darwin_amd64, linux_amd64, linux_arm64, windows_amd64"
    echo "  Branch:    $(git branch --show-current)"
    echo "  Commit:    $(git rev-parse --short HEAD)"
    echo ""

    if [[ "$dry_run" == true ]]; then
        log_warn "Dry run - no changes made"
        echo "Would run:"
        echo "  git tag -a $tag -m 'Release $tag'"
        echo "  git push origin $tag"
        exit 0
    fi

    read -p "Create and push tag $tag? [y/N] " -n 1 -r
    echo
    [[ ! $REPLY =~ ^[Yy]$ ]] && exit 1

    # Create and push tag
    log_info "Creating tag $tag..."
    git tag -a "$tag" -m "Release $tag"

    log_info "Pushing tag to origin..."
    git push origin "$tag"

    log_success "Tag $tag pushed!"
    echo ""
    echo "CI will now build all platforms. Monitor progress at:"
    echo "  https://github.com/thesyncim/libgowebrtc/actions"
    echo ""
    echo "Once complete, the release will be available at:"
    echo "  https://github.com/thesyncim/libgowebrtc/releases/tag/$tag"
}

main "$@"

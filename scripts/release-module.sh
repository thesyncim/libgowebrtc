#!/bin/bash
#
# Release script for libgowebrtc module tags.
#
# Creates semantic-version module tags (vX.Y.Z) and optionally pushes them.
# A GitHub Actions workflow will validate the tagged commit and publish the
# GitHub release once checks pass.
#
# Examples:
#   ./scripts/release-module.sh patch --dry-run
#   ./scripts/release-module.sh minor --push
#   ./scripts/release-module.sh v0.1.0 --push

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

log_info()    { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error()   { echo -e "\033[0;31m[ERROR]\033[0m $1"; }
log_warn()    { echo -e "\033[0;33m[WARN]\033[0m $1"; }

show_help() {
    cat << 'EOF'
Module Release Script
=====================

Creates a semantic-version module tag for libgowebrtc.
Use this for Go module/API releases. For binary shim asset releases, use
./scripts/release.sh instead.

Usage: ./scripts/release-module.sh [major|minor|patch|VERSION] [OPTIONS]

Arguments:
  major|minor|patch   Bump the latest stable module tag
  VERSION             Explicit module version, with or without leading v

Options:
  --target REF        Commit-ish to tag (default: origin/main)
  --notes TEXT        Annotated tag message (default: "libgowebrtc vX.Y.Z")
  --push              Push the new tag to origin
  --dry-run           Show what would happen without creating the tag
  --help              Show this help

Notes:
  - Module releases use tags like v0.1.0, v0.2.0, v1.0.0.
  - Shim/binary releases use tags like shim-vX.Y.Z and are separate.
  - If no stable module tag exists yet, the first suggested release is v0.1.0.
EOF
}

latest_module_tag() {
    local tags
    tags="$(git tag -l 'v*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' || true)"
    if [[ -z "$tags" ]]; then
        return
    fi
    printf '%s\n' "$tags" | sort -V | tail -1
}

normalize_version() {
    local raw="$1"
    if [[ "$raw" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
        echo "$raw"
        return
    fi
    if [[ "$raw" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
        echo "v$raw"
        return
    fi
    log_error "Invalid version '$raw'. Use semantic versions like v0.1.0 or 0.1.0."
    exit 1
}

suggest_bump() {
    local bump="$1"
    local latest
    latest="$(latest_module_tag)"

    if [[ -z "$latest" ]]; then
        echo "v0.1.0"
        return
    fi

    local version="${latest#v}"
    local major minor patch
    IFS='.' read -r major minor patch <<< "$version"

    case "$bump" in
        patch)
            patch=$((patch + 1))
            ;;
        minor)
            minor=$((minor + 1))
            patch=0
            ;;
        major)
            major=$((major + 1))
            minor=0
            patch=0
            ;;
        *)
            log_error "Unknown bump '$bump'. Use major, minor, patch, or an explicit version."
            exit 1
            ;;
    esac

    echo "v${major}.${minor}.${patch}"
}

main() {
    local dry_run=false
    local push_tag=false
    local target_ref="origin/main"
    local notes=""
    local selector=""

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --target)
                target_ref="$2"
                shift 2
                ;;
            --notes)
                notes="$2"
                shift 2
                ;;
            --push)
                push_tag=true
                shift
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            --help)
                show_help
                exit 0
                ;;
            *)
                if [[ -n "$selector" ]]; then
                    log_error "Only one bump selector or explicit version may be provided."
                    exit 1
                fi
                selector="$1"
                shift
                ;;
        esac
    done

    cd "$PROJECT_ROOT"

    local latest
    latest="$(latest_module_tag)"
    log_info "Latest stable module release: ${latest:-none}"

    if [[ -z "$selector" ]]; then
        selector="patch"
    fi

    local tag=""
    case "$selector" in
        major|minor|patch)
            tag="$(suggest_bump "$selector")"
            ;;
        *)
            tag="$(normalize_version "$selector")"
            ;;
    esac

    if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
        log_error "Tag $tag already exists"
        exit 1
    fi

    if [[ -z "$notes" ]]; then
        notes="libgowebrtc $tag"
    fi

    echo
    log_info "Module release summary:"
    echo "  Tag:       $tag"
    echo "  Latest:    ${latest:-none}"
    echo "  Target:    $target_ref"
    echo "  Commit:    $(git rev-parse --short "$target_ref")"
    echo "  Push:      $push_tag"
    echo

    if [[ "$dry_run" == true ]]; then
        log_warn "Dry run - no changes made"
        echo "Would run:"
        echo "  git tag -a $tag $target_ref -m \"$notes\""
        if [[ "$push_tag" == true ]]; then
            echo "  git push origin $tag"
        fi
        exit 0
    fi

    git tag -a "$tag" "$target_ref" -m "$notes"
    log_success "Created tag $tag"

    if [[ "$push_tag" == true ]]; then
        git push origin "$tag"
        log_success "Pushed tag $tag"
        echo
        echo "GitHub Actions will now validate the tagged commit and create the release."
        echo "Monitor progress at:"
        echo "  https://github.com/thesyncim/libgowebrtc/actions"
    else
        log_info "Tag created locally only. Push it with:"
        echo "  git push origin $tag"
    fi
}

main "$@"

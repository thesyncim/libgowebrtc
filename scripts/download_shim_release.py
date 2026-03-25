#!/usr/bin/env python3
"""Download a published shim archive from the embedded manifest and verify it."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import pathlib
import shutil
import sys
import tarfile
import tempfile
import urllib.error
import urllib.request


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Download and extract a published libwebrtc shim artifact."
    )
    parser.add_argument("--platform", required=True, help="Target platform key from the manifest")
    parser.add_argument("--output-dir", required=True, help="Directory to extract the artifact into")
    parser.add_argument(
        "--manifest",
        default="internal/ffi/shim_manifest.json",
        help="Path to the embedded shim manifest",
    )
    parser.add_argument(
        "--flavor",
        default="basic",
        help="Manifest flavor to use (default: basic)",
    )
    parser.add_argument(
        "--release-tag",
        default="",
        help="Override the release tag from the manifest",
    )
    parser.add_argument(
        "--base-url",
        default="",
        help="Override the base URL from the manifest",
    )
    parser.add_argument(
        "--archive-out",
        default="",
        help="Optional path to also copy the downloaded tarball to",
    )
    return parser.parse_args()


def resolve_sha256(base_url: str, release_tag: str, asset_name: str, expected_sha256: str) -> str:
    expected_sha256 = expected_sha256.strip().lower()
    if expected_sha256:
        return expected_sha256

    sha_url = f"{base_url}/{release_tag}/{asset_name}.sha256"
    request = urllib.request.Request(
        sha_url,
        headers={"User-Agent": "libgowebrtc-shim-downloader"},
    )
    try:
        with urllib.request.urlopen(request) as response:
            body = response.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as exc:
        raise SystemExit(f"Failed to download {sha_url}: HTTP {exc.code}") from exc
    except urllib.error.URLError as exc:
        raise SystemExit(f"Failed to download {sha_url}: {exc.reason}") from exc

    fields = body.split()
    if not fields:
        raise SystemExit(f"Checksum file was empty for {asset_name}: {sha_url}")

    digest = fields[0].strip().lower()
    if len(digest) != 64 or any(ch not in "0123456789abcdef" for ch in digest):
        raise SystemExit(f"Checksum file did not start with a valid SHA256 digest for {asset_name}: {sha_url}")
    return digest


def sha256_file(path: pathlib.Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def safe_extract(archive: tarfile.TarFile, output_dir: pathlib.Path) -> None:
    root = output_dir.resolve()
    for member in archive.getmembers():
        destination = (output_dir / member.name).resolve()
        if destination != root and not str(destination).startswith(f"{root}{os.sep}"):
            raise SystemExit(f"Refusing to extract unsafe path from archive: {member.name}")
    archive.extractall(output_dir)


def main() -> int:
    args = parse_args()

    manifest_path = pathlib.Path(args.manifest)
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    flavors = manifest["flavors"]
    if args.flavor not in flavors:
        raise SystemExit(f"Unknown flavor {args.flavor!r} in {manifest_path}")

    flavor = flavors[args.flavor]
    assets = flavor["assets"]
    if args.platform not in assets:
        raise SystemExit(f"Unknown platform {args.platform!r} in {manifest_path}")

    asset = assets[args.platform]
    asset_name = asset["file"]
    release_tag = args.release_tag or flavor["release_tag"]
    base_url = (args.base_url or manifest["base_url"]).rstrip("/")
    expected_sha256 = resolve_sha256(base_url, release_tag, asset_name, asset.get("sha256", ""))
    download_url = f"{base_url}/{release_tag}/{asset_name}"

    output_dir = pathlib.Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    archive_out = pathlib.Path(args.archive_out) if args.archive_out else None

    with tempfile.TemporaryDirectory(prefix="libgowebrtc-shim-") as tmp_dir:
        archive_path = pathlib.Path(tmp_dir) / asset_name
        request = urllib.request.Request(
            download_url,
            headers={"User-Agent": "libgowebrtc-shim-downloader"},
        )
        try:
            with urllib.request.urlopen(request) as response, archive_path.open("wb") as handle:
                shutil.copyfileobj(response, handle)
        except urllib.error.HTTPError as exc:
            raise SystemExit(f"Failed to download {download_url}: HTTP {exc.code}") from exc
        except urllib.error.URLError as exc:
            raise SystemExit(f"Failed to download {download_url}: {exc.reason}") from exc

        actual_sha256 = sha256_file(archive_path)
        if actual_sha256 != expected_sha256:
            raise SystemExit(
                f"Checksum mismatch for {asset_name}: expected {expected_sha256}, got {actual_sha256}"
            )

        if archive_out is not None:
            archive_out.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(archive_path, archive_out)

        with tarfile.open(archive_path, "r:gz") as archive:
            safe_extract(archive, output_dir)

    print(f"downloaded {download_url}")
    print(f"verified sha256 {expected_sha256}")
    print(f"extracted to {output_dir}")
    return 0


if __name__ == "__main__":
    sys.exit(main())

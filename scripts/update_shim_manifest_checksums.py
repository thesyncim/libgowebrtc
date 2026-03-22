#!/usr/bin/env python3

import argparse
import json
import pathlib
import sys


def parse_sha256_file(path: pathlib.Path) -> str:
    text = path.read_text(encoding="utf-8").strip()
    fields = text.split()
    if not fields:
        raise ValueError(f"{path} is empty")
    digest = fields[0].lower()
    if len(digest) != 64 or any(ch not in "0123456789abcdef" for ch in digest):
        raise ValueError(f"{path} does not start with a valid SHA256 digest")
    return digest


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Backfill SHA256 values in internal/ffi/shim_manifest.json from release .sha256 files."
    )
    parser.add_argument(
        "--manifest",
        default="internal/ffi/shim_manifest.json",
        help="Path to shim manifest JSON (default: %(default)s)",
    )
    parser.add_argument(
        "--release-dir",
        required=True,
        help="Directory containing libwebrtc_shim_*.tar.gz.sha256 files",
    )
    parser.add_argument(
        "--flavor",
        default="basic",
        help="Manifest flavor to update (default: %(default)s)",
    )
    args = parser.parse_args()

    manifest_path = pathlib.Path(args.manifest)
    release_dir = pathlib.Path(args.release_dir)

    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    try:
        assets = manifest["flavors"][args.flavor]["assets"]
    except KeyError as exc:
        raise SystemExit(f"Missing manifest key: {exc}") from exc

    missing = []
    for platform, asset in assets.items():
        sha_path = release_dir / f"{asset['file']}.sha256"
        if not sha_path.is_file():
            missing.append(f"{platform}: {sha_path}")
            continue
        asset["sha256"] = parse_sha256_file(sha_path)

    if missing:
        print("Missing checksum files:", file=sys.stderr)
        for item in missing:
            print(f"  {item}", file=sys.stderr)
        return 1

    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

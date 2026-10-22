#!/usr/bin/env python3
"""Package committed deployment files for an already published Nero image."""

import argparse
import hashlib
import io
import json
import re
import subprocess
import tarfile
from datetime import datetime, timezone
from pathlib import Path


def git(*arguments):
    result = subprocess.run(
        ["git", *arguments], capture_output=True, check=False
    )
    if result.returncode:
        raise ValueError(result.stderr.decode("utf-8", errors="replace").strip())
    return result.stdout


def package(args):
    if not re.fullmatch(r"[a-z0-9][a-z0-9._:/-]*@sha256:[0-9a-f]{64}", args.image):
        raise ValueError("--image must be an image reference pinned by @sha256:<64 hex digits>")

    commit = git("rev-parse", "--verify", "--end-of-options", args.commit + "^{commit}").decode().strip()
    now = datetime.now(timezone.utc)
    release_id = args.release_id or f"{now:%Y%m%d-%H%M%S}-{commit[:12]}"
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,79}", release_id):
        raise ValueError("--release-id must be 1-80 letters, digits, dots, hyphens or underscores, starting with a letter or digit")

    required = {
        "deploy/compose.yaml": "compose.yaml",
        "deploy/runtime.example.env": "runtime.example.env",
    }
    files = {}
    entries = git("ls-tree", "-r", "-z", "--full-tree", commit, "--", *required, "migrations")
    for entry in entries.split(b"\0"):
        if not entry:
            continue
        metadata, raw_path = entry.split(b"\t", 1)
        mode, kind, blob = metadata.decode().split()
        path = raw_path.decode("utf-8")
        migration = re.fullmatch(r"migrations/[0-9]+_[A-Za-z0-9_-]+\.(up|down)\.sql", path)
        if path not in required and not migration:
            if path.startswith("migrations/") and path.endswith(".sql"):
                raise ValueError(f"Unsupported migration filename: {path}")
            continue
        if kind != "blob" or mode not in ("100644", "100755"):
            raise ValueError(f"Release input must be a regular committed file: {path}")
        files[required.get(path, path)] = git("cat-file", "blob", blob)

    missing = set(required.values()) - files.keys()
    if missing:
        raise ValueError(f"Commit {commit} is missing: {', '.join(sorted(missing))}")
    if not any(path.endswith(".up.sql") for path in files):
        raise ValueError("The commit contains no up migrations")

    manifest = {
        "schemaVersion": 1,
        "releaseId": release_id,
        "createdAtUtc": now.isoformat().replace("+00:00", "Z"),
        "sourceCommit": commit,
        "image": args.image,
        "buildUrl": args.build_url,
    }
    files["build.json"] = (json.dumps(manifest, indent=2) + "\n").encode()
    files["checksums.sha256"] = "".join(
        f"{hashlib.sha256(data).hexdigest()}  {name}\n"
        for name, data in sorted(files.items())
    ).encode()

    repo = Path(git("rev-parse", "--show-toplevel").decode().strip())
    output = Path(args.output_dir) if args.output_dir else repo / "out" / "releases"
    archive = output / f"{release_id}.tar.gz"
    sidecar = output / f"{release_id}.tar.gz.sha256"
    if archive.exists() or sidecar.exists():
        raise ValueError(f"Release output already exists: {release_id}; choose a new ID")
    output.mkdir(parents=True, exist_ok=True)
    created = []
    try:
        with archive.open("xb") as destination:
            created.append(archive)
            with tarfile.open(fileobj=destination, mode="w:gz") as bundle:
                for name, data in sorted(files.items()):
                    info = tarfile.TarInfo(name)
                    info.size = len(data)
                    info.mode = 0o644
                    info.mtime = int(now.timestamp())
                    bundle.addfile(info, io.BytesIO(data))
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        with sidecar.open("x", encoding="ascii", newline="\n") as checksum:
            created.append(sidecar)
            checksum.write(f"{digest}  {archive.name}\n")
    except BaseException:
        for path in reversed(created):
            path.unlink(missing_ok=True)
        raise
    print(f"Commit: {commit}\nImage: {args.image}\nArchive: {archive}\nSHA256: {digest}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--commit", required=True, help="Git commit/ref from the image's CI build")
    parser.add_argument("--image", required=True, help="Full image@sha256:... from the same build")
    parser.add_argument("--release-id", help="Unique output ID; default: UTC timestamp and commit")
    parser.add_argument("--build-url", help="URL of the CI run that published the image")
    parser.add_argument("--output-dir", help="Output directory; default: out/releases in the repository")
    args = parser.parse_args()
    try:
        package(args)
    except (OSError, ValueError, tarfile.TarError) as error:
        parser.exit(1, f"Error: {error}\n")


if __name__ == "__main__":
    main()

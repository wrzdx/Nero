import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[1] / "package-release.py"
IMAGE = "ghcr.io/wrzdx/nero@sha256:" + "a" * 64


class PackageReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.repo = Path(self.temporary.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Release test")
        self.git("config", "user.email", "release-test@example.invalid")
        for path, content in {
            "deploy/compose.yaml": "name: original\n",
            "deploy/runtime.example.env": "JWT_SECRET=\n",
            "migrations/000001_init.up.sql": "SELECT 1;\n",
            "migrations/000001_init.down.sql": "SELECT 2;\n",
            ".env": "JWT_SECRET=must-not-be-packaged\n",
        }.items():
            destination = self.repo / path
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_text(content)
        self.git("add", ".")
        self.git("commit", "-qm", "Fixture")
        self.commit = self.git("rev-parse", "HEAD").strip()

    def git(self, *arguments):
        return subprocess.check_output(["git", *arguments], cwd=self.repo, text=True)

    def run_package(self, *extra):
        return subprocess.run(
            [sys.executable, str(SCRIPT), "--commit", self.commit, "--image", IMAGE,
             "--release-id", "test-release", *extra],
            cwd=self.repo, capture_output=True, text=True,
        )

    def test_committed_snapshot_and_checksums_exclude_secrets_and_worktree(self):
        (self.repo / "deploy/compose.yaml").write_text("name: uncommitted\n")
        (self.repo / "runtime.env").write_text("JWT_SECRET=also-private\n")
        result = self.run_package("--build-url", "https://example.invalid/build/1")
        self.assertEqual(result.returncode, 0, result.stderr)
        archive = self.repo / "out/releases/test-release.tar.gz"
        with tarfile.open(archive) as bundle:
            files = {entry.name: bundle.extractfile(entry).read() for entry in bundle}
        self.assertEqual(set(files), {"compose.yaml", "runtime.example.env", "build.json",
                                    "checksums.sha256", "migrations/000001_init.up.sql",
                                    "migrations/000001_init.down.sql"})
        self.assertEqual(files["compose.yaml"], b"name: original\n")
        manifest = json.loads(files["build.json"])
        self.assertEqual(manifest["sourceCommit"], self.commit)
        self.assertEqual(manifest["image"], IMAGE)
        self.assertEqual(manifest["buildUrl"], "https://example.invalid/build/1")
        checksums = dict(line.split("  ", 1)[::-1] for line in files["checksums.sha256"].decode().splitlines())
        self.assertEqual(set(checksums), set(files) - {"checksums.sha256"})
        for name, digest in checksums.items():
            self.assertEqual(hashlib.sha256(files[name]).hexdigest(), digest)
        sidecar = archive.with_name(archive.name + ".sha256").read_text()
        self.assertEqual(sidecar, f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n")

    def test_existing_release_is_never_overwritten(self):
        self.assertEqual(self.run_package().returncode, 0)
        archive = self.repo / "out/releases/test-release.tar.gz"
        original = archive.read_bytes()
        self.assertNotEqual(self.run_package().returncode, 0)
        self.assertEqual(archive.read_bytes(), original)

    def test_invalid_image_commit_or_release_id_is_rejected(self):
        for arguments in (("--image", "nero:latest"), ("--commit", "missing-ref"),
                          ("--release-id", "../escape")):
            with self.subTest(arguments=arguments):
                self.assertNotEqual(self.run_package(*arguments).returncode, 0)
        self.assertFalse((self.repo / "out").exists())

    def test_missing_input_and_symlink_are_rejected(self):
        self.git("rm", "deploy/runtime.example.env")
        self.git("commit", "-qm", "Remove template")
        self.commit = self.git("rev-parse", "HEAD").strip()
        self.assertNotEqual(self.run_package().returncode, 0)
        (self.repo / "deploy/runtime.example.env").symlink_to("../.env")
        self.git("add", "deploy/runtime.example.env")
        self.git("commit", "-qm", "Link template")
        self.commit = self.git("rev-parse", "HEAD").strip()
        self.assertNotEqual(self.run_package().returncode, 0)
        self.assertFalse((self.repo / "out").exists())


if __name__ == "__main__":
    unittest.main()

import importlib.util
import pathlib
import subprocess
import unittest
from unittest.mock import patch

MODULE = pathlib.Path(__file__).parents[1] / "sync-secret-openbao-to-github.py"
spec = importlib.util.spec_from_file_location("sync_secret", MODULE)
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)


class SyncTests(unittest.TestCase):
    def test_single_read_preserves_bytes_and_uses_stdin(self):
        with patch.object(mod.subprocess, "run") as run:
            run.return_value = subprocess.CompletedProcess([], 0, b"fixture\n")
            mod.sync(
                "mark8ly/app/mark8ly-fixture",
                "FIXTURE",
                "tesserix/mark8ly",
            )
        self.assertEqual(run.call_count, 2)
        self.assertEqual(run.call_args.kwargs["input"], b"fixture\n")
        self.assertNotIn("fixture", " ".join(run.call_args.args[0]))

    def test_empty_or_failed_reads_never_write(self):
        for result in (
            subprocess.CompletedProcess([], 0, b""),
            subprocess.CompletedProcess([], 1, b""),
        ):
            with patch.object(mod.subprocess, "run", return_value=result) as run:
                with self.assertRaises(RuntimeError):
                    mod.sync(
                        "mark8ly/app/mark8ly-fixture",
                        "FIXTURE",
                        "tesserix/mark8ly",
                    )
                self.assertEqual(run.call_count, 1)

    def test_rejects_other_product_and_traversal(self):
        for path in ("other/app/key", "mark8ly/app/../other", "prod-mark8ly-key"):
            with patch.object(mod.subprocess, "run") as run:
                with self.assertRaises(ValueError):
                    mod.sync(path, "FIXTURE", "tesserix/mark8ly")
                run.assert_not_called()

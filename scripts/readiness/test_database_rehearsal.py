"""Safety boundaries must reject working projects and remote Docker before I/O."""
import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("database_rehearsal", Path(__file__).with_name("database_rehearsal.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class SafetyTests(unittest.TestCase):
    def reject(self, project, endpoint):
        argv = ["database_rehearsal.py", "--project", project, "--evidence", "/unused"]
        with patch("sys.argv", argv), patch.dict(os.environ, {"DOCKER_HOST": endpoint}), \
                patch.object(module.subprocess, "run") as execute, contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(SystemExit) as caught:
                module.main()
            self.assertEqual(caught.exception.code, 2)
            execute.assert_not_called()

    def test_working_project_refused_before_docker(self):
        for name in ["production", "aba-demo10d-local", "aba-private-staging", "aba-readiness-integration-"]:
            with self.subTest(project=name):
                self.reject(name, "unix:///unused.sock")

    def test_remote_endpoint_refused_before_docker(self):
        for endpoint in ["tcp://127.0.0.1:2375", "ssh://admin@example.test", "http://localhost:2375"]:
            with self.subTest(endpoint=endpoint):
                self.reject("aba-readiness-integration-synthetic", endpoint)

    def test_optimized_python_still_refuses_missing_synthetic_marker(self):
        namespace = {"__name__": "optimized_rehearsal", "__file__": module.__file__}
        exec(compile(Path(module.__file__).read_text(), module.__file__, "exec", optimize=2), namespace)
        metadata = [{"HostConfig": {}, "Mounts": [], "NetworkSettings": {"Networks": {"isolated": {}}}}]
        responses = [b"synthetic-container", json.dumps(metadata).encode(), b'[{"Internal":true}]', b"wrong-marker"]
        argv = ["database_rehearsal.py", "--project", "aba-readiness-integration-synthetic", "--evidence", "/unused"]
        with patch("sys.argv", argv), patch.dict(os.environ, {"DOCKER_HOST": "unix:///unused.sock"}), \
                patch.dict(namespace, {"run": unittest.mock.Mock(side_effect=responses)}):
            with self.assertRaisesRegex(RuntimeError, "synthetic safety marker missing"):
                namespace["main"]()
            self.assertEqual(namespace["run"].call_count, 4)  # No CREATE DATABASE / dump / filesystem mutation.


if __name__ == "__main__":
    unittest.main()

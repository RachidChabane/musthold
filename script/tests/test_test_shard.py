"""script/test-shard: every package of a module runs in exactly one shard."""

from __future__ import annotations

import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "script" / "test-shard"
EXIT_USAGE = 2


def run(*args: str, cwd: Path = ROOT) -> subprocess.CompletedProcess[str]:
    return subprocess.run([*args], cwd=cwd, capture_output=True, text=True, check=False)


def lines(text: str) -> list[str]:
    return [line for line in text.splitlines() if line]


class ShardPartitionTest(unittest.TestCase):
    def test_the_shards_of_each_module_partition_its_packages(self) -> None:
        for module in lines(run(str(ROOT / "script" / "modules")).stdout):
            with self.subTest(module=module):
                shards = lines(run(str(SCRIPT), "--list", module).stdout)
                self.assertEqual(shards[-1], "rest")
                drawn: list[str] = []
                for shard in shards:
                    listed = run(str(SCRIPT), "--packages", module, shard)
                    self.assertEqual(listed.returncode, 0, listed.stderr)
                    drawn += lines(listed.stdout)
                packages = lines(run("go", "list", "./...", cwd=ROOT / module).stdout)
                self.assertEqual(sorted(drawn), sorted(packages))

    def test_the_discriminating_power_module_runs_its_two_long_packages_alone(self) -> None:
        self.assertEqual(lines(run(str(SCRIPT), "--list", "./discpower").stdout), ["cli", "e2e", "rest"])

    def test_an_unknown_shard_is_refused(self) -> None:
        refused = run(str(SCRIPT), "--packages", "./discpower", "nope")
        self.assertEqual(refused.returncode, EXIT_USAGE)
        self.assertIn("has no shard nope", refused.stderr)


if __name__ == "__main__":
    unittest.main()

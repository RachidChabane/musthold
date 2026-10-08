"""Tests of script/third-party-notices. Run: python3 -m unittest discover -s script/tests"""

import importlib.machinery
import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent.parent / "third-party-notices"


def load_script():
    loader = importlib.machinery.SourceFileLoader("third_party_notices", str(SCRIPT))
    spec = importlib.util.spec_from_loader("third_party_notices", loader)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    loader.exec_module(module)
    return module


notices = load_script()

MIT_TEXT = """MIT License

Copyright (c) 2026 Someone

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
"""
BSD_2_TEXT = """Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:
1. Redistributions of source code must retain the above copyright notice.
2. Redistributions in binary form must reproduce the above copyright notice.
"""
BSD_3_TEXT = BSD_2_TEXT + "3. Neither the name of the copyright holder nor the names of its contributors may be used.\n"
APACHE_TEXT = "Apache License\nVersion 2.0, January 2004\nhttp://www.apache.org/licenses/\n"
MPL_TEXT = "Mozilla Public License Version 2.0\n==================================\n"
ISC_TEXT = (
    "Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee "
    "is hereby granted, provided that the above copyright notice and this permission notice appear in all copies.\n"
)
UNRECOGNISED_TEXT = "All rights reserved. Ask the author.\n"


class ClassifyLicenseText(unittest.TestCase):
    def test_known_texts(self):
        cases = {
            MIT_TEXT: "MIT",
            BSD_2_TEXT: "BSD-2-Clause",
            BSD_3_TEXT: "BSD-3-Clause",
            APACHE_TEXT: "Apache-2.0",
            MPL_TEXT: "MPL-2.0",
            ISC_TEXT: "ISC",
        }
        for text, expected in cases.items():
            with self.subTest(expected=expected):
                self.assertEqual(notices.classify_license_text(text), expected)

    def test_unrecognised_text_is_unknown(self):
        self.assertEqual(notices.classify_license_text(UNRECOGNISED_TEXT), notices.UNKNOWN)

    def test_line_wrapping_does_not_change_the_class(self):
        self.assertEqual(notices.classify_license_text(MIT_TEXT.replace("\n", "\n\n")), "MIT")


class ClassifyFiles(unittest.TestCase):
    def write(self, directory: Path, name: str, text: str) -> Path:
        path = directory / name
        path.write_text(text)
        return path

    def test_two_different_texts_are_joined_in_sorted_order(self):
        with tempfile.TemporaryDirectory() as raw:
            directory = Path(raw)
            self.write(directory, "LICENSE", APACHE_TEXT)
            self.write(directory, "LICENSE.docs", MIT_TEXT)
            self.assertEqual(notices.classify_files(notices.license_files(directory)), "Apache-2.0 AND MIT")

    def test_one_unrecognised_text_makes_the_whole_unknown(self):
        with tempfile.TemporaryDirectory() as raw:
            directory = Path(raw)
            self.write(directory, "LICENSE", MIT_TEXT)
            self.write(directory, "COPYING", UNRECOGNISED_TEXT)
            self.assertEqual(notices.classify_files(notices.license_files(directory)), notices.UNKNOWN)

    def test_no_licence_file_is_unknown(self):
        with tempfile.TemporaryDirectory() as raw:
            self.assertEqual(notices.classify_files(notices.license_files(Path(raw))), notices.UNKNOWN)

    def test_source_files_named_like_a_licence_are_not_licence_files(self):
        with tempfile.TemporaryDirectory() as raw:
            directory = Path(raw)
            self.write(directory, "license.go", "package x")
            self.assertEqual(notices.license_files(directory), [])


class DeclaredLicense(unittest.TestCase):
    def test_expression_wins_over_classifiers(self):
        text = "Name: x\nLicense-Expression: MIT\nClassifier: License :: OSI Approved :: BSD License\n"
        self.assertEqual(notices.declared_license(text), "MIT")

    def test_classifiers_are_sorted_and_joined(self):
        text = (
            "Name: x\nClassifier: License :: OSI Approved :: BSD License\n"
            "Classifier: License :: OSI Approved :: Apache Software License\n"
        )
        self.assertEqual(notices.declared_license(text), "Apache Software License; BSD License")

    def test_short_license_field_is_used_without_classifiers(self):
        self.assertEqual(notices.declared_license("Name: x\nLicense: ISC\n"), "ISC")

    def test_long_license_field_is_not_a_licence_name(self):
        self.assertEqual(notices.declared_license("Name: x\nLicense: " + "word " * 40 + "\n"), notices.UNKNOWN)

    def test_nothing_declared_is_unknown(self):
        self.assertEqual(notices.declared_license("Name: x\n"), notices.UNKNOWN)


class InstalledLicenses(unittest.TestCase):
    def make_distribution(self, root: Path, directory: str, metadata: str, license_text: str = "") -> None:
        dist_info = root / directory
        (dist_info / "licenses").mkdir(parents=True)
        (dist_info / "METADATA").write_text(metadata)
        if license_text:
            (dist_info / "licenses" / "LICENSE").write_text(license_text)

    def test_index_normalises_names_and_reads_bundled_licence_texts(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            self.make_distribution(root, "Some_Pkg-1.0.dist-info", "Name: Some_Pkg\nVersion: 1.0\n", MIT_TEXT)
            self.make_distribution(root, "other-2.0.dist-info", "Name: other\nVersion: 2.0\nLicense-Expression: ISC\n")
            index = notices.index_installed_licenses([root])
            self.assertEqual(index, {("some-pkg", "1.0"): "MIT", ("other", "2.0"): "ISC"})

    def test_first_directory_wins(self):
        with tempfile.TemporaryDirectory() as first_raw, tempfile.TemporaryDirectory() as second_raw:
            for raw, value in ((first_raw, "MIT"), (second_raw, "ISC")):
                self.make_distribution(Path(raw), "p-1.dist-info", f"Name: p\nVersion: 1\nLicense-Expression: {value}\n")
            index = notices.index_installed_licenses([Path(first_raw), Path(second_raw)])
            self.assertEqual(index[("p", "1")], "MIT")

    def test_undeterminable_distribution_is_left_out(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            self.make_distribution(root, "p-1.dist-info", "Name: p\nVersion: 1\n")
            self.assertEqual(notices.index_installed_licenses([root]), {})


class LockedPackages(unittest.TestCase):
    def test_registry_packages_only(self):
        lock = (
            'version = 1\n\n[[package]]\nname = "Some_Pkg"\nversion = "1.0"\nsource = { registry = "https://pypi.org/simple" }\n\n'
            '[[package]]\nname = "own"\nversion = "0.1.0"\nsource = { editable = "." }\n'
        )
        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "uv.lock"
            path.write_text(lock)
            self.assertEqual(notices.locked_packages(path), {("some-pkg", "1.0")})


class Rendering(unittest.TestCase):
    GO = [
        notices.GoModule("example.com/a", "v1.0.0", "MPL-2.0", notices.SCOPE_BINARY),
        notices.GoModule("example.com/b", "v2.0.0", "MIT", notices.SCOPE_DEVELOPMENT),
    ]
    PYTHON = [notices.PythonPackage("pkg", "1.0", notices.UNKNOWN, ("bench",))]

    def test_render_is_deterministic(self):
        self.assertEqual(notices.render(self.GO, self.PYTHON), notices.render(self.GO, self.PYTHON))

    def test_mpl_modules_are_called_out(self):
        text = notices.render(self.GO, self.PYTHON)
        self.assertIn("### MPL-2.0 modules\n", text)
        self.assertIn("- `example.com/a` v1.0.0 (binary)", text)

    def test_undeterminable_licence_stays_unknown(self):
        self.assertIn("| `pkg` | `1.0` | UNKNOWN | bench |", notices.render(self.GO, self.PYTHON))

    def test_recorded_licences_read_back_what_render_wrote(self):
        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "NOTICES.md"
            path.write_text(notices.render(self.GO, self.PYTHON))
            recorded = notices.read_recorded(path)
        self.assertEqual(recorded[("go", "example.com/a", "v1.0.0")], "MPL-2.0")
        self.assertEqual(recorded[("python", "pkg", "1.0")], notices.UNKNOWN)

    def test_a_licence_missing_from_every_source_falls_back_to_the_recorded_one(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            lock = root / "bench"
            lock.mkdir()
            (lock / "uv.lock").write_text(
                '[[package]]\nname = "pkg"\nversion = "1.0"\nsource = { registry = "https://pypi.org/simple" }\n'
            )
            original = notices.PYTHON_LOCK_FILES
            notices.PYTHON_LOCK_FILES = ("bench/uv.lock",)
            try:
                packages = notices.python_packages(root, [], {("python", "pkg", "1.0"): "MIT"})
            finally:
                notices.PYTHON_LOCK_FILES = original
        self.assertEqual(packages, [notices.PythonPackage("pkg", "1.0", "MIT", ("bench",))])


if __name__ == "__main__":
    unittest.main()

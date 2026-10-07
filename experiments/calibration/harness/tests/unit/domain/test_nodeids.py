from __future__ import annotations

from calibration.domain.nodeids import NODE_SEPARATOR, nodeid_of

FILE = "tests/test_parking.py"
MODULE = "tests.test_parking"
TEST_NAME = "test_fee_is_rounded"


class TestModuleLevelTests:
    def test_the_class_part_is_left_out(self) -> None:
        assert nodeid_of(FILE, MODULE, TEST_NAME) == f"{FILE}{NODE_SEPARATOR}{TEST_NAME}"

    def test_a_nested_package_path_is_reduced_to_the_module(self) -> None:
        assert nodeid_of("tests/unit/test_fee.py", "tests.unit.test_fee", TEST_NAME) == (
            f"tests/unit/test_fee.py::{TEST_NAME}"
        )


class TestClassTests:
    def test_the_class_follows_the_file(self) -> None:
        assert nodeid_of(FILE, f"{MODULE}.TestFee", TEST_NAME) == f"{FILE}::TestFee::{TEST_NAME}"

    def test_nested_classes_keep_their_order(self) -> None:
        assert nodeid_of(FILE, f"{MODULE}.TestFee.TestRounding", TEST_NAME) == (
            f"{FILE}::TestFee::TestRounding::{TEST_NAME}"
        )

    def test_a_parametrized_name_is_kept_verbatim(self) -> None:
        assert nodeid_of(FILE, f"{MODULE}.TestFee", "test_fee[5-0.50]") == f"{FILE}::TestFee::test_fee[5-0.50]"

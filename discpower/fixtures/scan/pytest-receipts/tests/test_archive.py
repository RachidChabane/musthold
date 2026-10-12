import pytest

from receipts.archive import archive


@pytest.fixture
def batch():
    entries = [3, 4]
    yield entries
    archive(entries)


def test_batch_holds_two_entries(batch):
    assert len(batch) == 2


def test_archive_sums_the_positive_entries():
    assert archive([3, -1, 4]) == (7, 2)

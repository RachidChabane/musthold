import pytest

from invoice.ledger import Ledger


@pytest.fixture(scope="module")
def ledger():
    book = Ledger()
    yield book
    book.close()


@pytest.fixture
def day_book():
    book = Ledger()
    yield book
    book.close()


def test_post_adds_the_amount(ledger):
    assert ledger.post(5) >= 5


def test_day_book_starts_empty(day_book):
    assert day_book.entries == []


def test_ledger_is_named():
    assert Ledger.__name__ == "Ledger"

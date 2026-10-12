import threading
import time

from invoice.ledger import Ledger

ticking = threading.Event()
stop = threading.Event()


def test_ticker_starts():
    book = Ledger()

    def tick():
        while not stop.is_set():
            book.post(1)
            ticking.set()
            time.sleep(0.001)

    threading.Thread(target=tick, daemon=True).start()
    assert ticking.wait(5)


def test_ticker_keeps_its_own_book():
    time.sleep(0.05)
    assert Ledger().entries == []
    stop.set()

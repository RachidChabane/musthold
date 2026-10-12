ARCHIVE_LIMIT = 50


def archive(entries):
    kept = [entry for entry in entries if entry > 0]
    if len(kept) > ARCHIVE_LIMIT:
        kept = kept[-ARCHIVE_LIMIT:]
    total = sum(kept)
    return total, len(kept)

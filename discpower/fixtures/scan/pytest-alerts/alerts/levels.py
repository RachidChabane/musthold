LOUD_FROM = 40
MUTED_DROP = 10

_muted = set()


def mute(channel: str) -> None:
    _muted.add(channel)


def is_muted(channel: str) -> bool:
    return channel in _muted


def route(channel: str, level: int) -> int:
    if channel in _muted:
        return quiet_level(level)
    return level


def quiet_level(level: int) -> int:
    if level >= LOUD_FROM:
        return level - MUTED_DROP
    return 0

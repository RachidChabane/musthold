import sys


def clamp(value, low, high):
    if value < low:
        return low
    if value > high:
        return high
    return value


if sys.version_info >= (3, 12):

    def _comparator(left, right):
        if left > right:
            return 1
        return 0

else:

    def _comparator(left, right):
        if left < right:
            return -1
        return 0


class Version:
    def __init__(self, major, minor):
        self.major = major
        self.minor = minor

    def bump(self, level):
        if level > 1:
            return Version(self.major + 1, 0)
        return Version(self.major, self.minor + 1)

from enum import Enum

MULTIPLIERS = {"domestic": 1.0, "europe": 1.8}


class Region(Enum):
    DOMESTIC = "domestic"
    EUROPE = "europe"


def zone_multiplier(zone: str) -> float:
    return MULTIPLIERS[zone]

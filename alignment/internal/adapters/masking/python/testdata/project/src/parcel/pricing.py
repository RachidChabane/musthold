"""Pricing of parcels."""

from dataclasses import dataclass

DEFAULT_RATE = 4
HEAVY_LIMIT_KG = 30.0


@dataclass
class Parcel:
    """A parcel to ship."""

    weight_kg: float
    zone: str = "domestic"

    def is_heavy(self) -> bool:
        return self.weight_kg > HEAVY_LIMIT_KG


class RateCard:
    base: int
    express_factor: float = 1.5

    def __init__(self, base: int = DEFAULT_RATE) -> None:
        self.base = base

    def quote(
        self,
        parcel: Parcel,  # the parcel to price
        *,
        express: bool = False,
    ) -> float:
        """Quote a parcel."""
        factor = self.express_factor if express else 1.0
        return self.base * parcel.weight_kg * factor


def line_total(quantity: int, unit_price: float) -> float:
    """Total of one order line."""
    return quantity * unit_price


def surcharge(parcel: Parcel, rate: float = 0.1) -> float:
    if parcel.is_heavy():
        return parcel.weight_kg * rate
    return 0.0

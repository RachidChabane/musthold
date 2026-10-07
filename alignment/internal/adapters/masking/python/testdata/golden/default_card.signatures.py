class Parcel:
    weight_kg: float
    zone: str
    def is_heavy(self) -> bool:
        ...

class RateCard:
    base: int
    express_factor: float
    def __init__(self, base: int = DEFAULT_RATE) -> None:
        ...
    def quote(
        self,
        parcel: Parcel,
        *,
        express: bool = False,
    ) -> float:
        ...

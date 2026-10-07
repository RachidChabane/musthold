class Parcel:
    weight_kg: float
    zone: str
    def is_heavy(self) -> bool:
        ...

def surcharge(parcel: Parcel, rate: float = 0.1) -> float:
    ...

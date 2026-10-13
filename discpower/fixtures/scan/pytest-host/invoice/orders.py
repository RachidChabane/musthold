def order_total(lines: list[tuple[int, int]]) -> int:
    total = 0
    for unit_price, quantity in lines:
        total += unit_price * quantity
    return total

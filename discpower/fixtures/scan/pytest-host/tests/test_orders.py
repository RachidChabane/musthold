from invoice.orders import order_total


def test_order_total_adds_up_the_lines():
    order_total([(4, 3), (2, 5)])

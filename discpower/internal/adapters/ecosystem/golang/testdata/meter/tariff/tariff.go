package tariff

// Table holds the price of each zone.
type Table map[string]int

// Standard is the table every tariff starts from.
func Standard() Table {
	return Table{"near": 3, "far": 8}
}

// Price is the price of a zone in the table, zero for an unknown zone.
func Price(table Table, zone string) int {
	price, known := table[zone]
	if !known {
		return 0
	}
	return price
}

// Surcharge adds the handling fee to a price.
func Surcharge(price int) int {
	return price + 2
}

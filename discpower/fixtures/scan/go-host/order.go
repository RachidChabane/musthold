package tally

// Line is one line of an order.
type Line struct {
	UnitPrice int
	Quantity  int
}

// OrderTotal adds up the lines of an order.
func OrderTotal(lines []Line) int {
	total := 0
	for _, line := range lines {
		total += line.UnitPrice * line.Quantity
	}
	return total
}

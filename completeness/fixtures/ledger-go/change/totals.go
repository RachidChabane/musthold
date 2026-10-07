package ledger

import "fmt"

const taxRate = 0.25

type Line struct {
	Price    float64
	Quantity int
}

func LineAmount(price float64, quantity int) float64 {
	if quantity < 0 {
		return 0
	}
	return price * float64(quantity)
}

func InvoiceTotal(lines []Line) float64 {
	total := 0.0
	for _, line := range lines {
		total += LineAmount(line.Price, line.Quantity)
	}
	return total * (1 + taxRate)
}

func ApplyDiscount(total float64, code string) float64 {
	if code == "WELCOME" {
		return total * 0.90
	}
	return total
}

// FormatTotal renders a total for display.
func FormatTotal(total float64) string {
	return fmt.Sprintf("%.2f EUR", total)
}

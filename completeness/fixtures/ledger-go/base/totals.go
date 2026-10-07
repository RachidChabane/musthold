package ledger

import "fmt"

const taxRate = 0.20

type Line struct {
	Price    float64
	Quantity int
}

func LineAmount(price float64, quantity int) float64 {
	return price * float64(quantity)
}

func InvoiceTotal(lines []Line) float64 {
	total := 0.0
	for _, line := range lines {
		total += LineAmount(line.Price, line.Quantity)
	}
	return total
}

func FormatTotal(total float64) string {
	return fmt.Sprintf("%.2f EUR", total)
}

package receipts

import "strconv"

// Format writes an amount in cents with its currency, the euro by default.
func Format(cents int, currency string) string {
	if currency == "" {
		currency = "EUR"
	}
	units, rest := cents/100, cents%100
	if rest < 10 {
		return strconv.Itoa(units) + ".0" + strconv.Itoa(rest) + " " + currency
	}
	return strconv.Itoa(units) + "." + strconv.Itoa(rest) + " " + currency
}

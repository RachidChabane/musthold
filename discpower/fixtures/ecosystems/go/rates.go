package parcel

const (
	FreeShippingFrom = 5000
	FlatFee          = 450
	MaxPercent       = 100
)

func LineTotal(unitPrice, quantity int) int {
	return unitPrice * quantity
}

func ClampPercent(percent int) int {
	return max(0, min(MaxPercent, percent))
}

func Discounted(total, percent int) int {
	kept := MaxPercent - ClampPercent(percent)
	return total * kept / MaxPercent
}

func ShippingFee(total int) int {
	if total >= FreeShippingFrom {
		return 0
	}
	return FlatFee
}

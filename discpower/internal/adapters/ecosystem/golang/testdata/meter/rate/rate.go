package rate

// Scale multiplies a value by a factor; a factor below one leaves it as it is.
func Scale(value, factor int) int {
	if factor < 1 {
		return value
	}
	return value * factor
}

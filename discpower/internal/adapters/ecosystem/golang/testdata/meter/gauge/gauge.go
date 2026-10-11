package gauge

// Reading turns a raw sensor value into a reading; a negative value reads as
// zero.
func Reading(raw int) int {
	if raw < 0 {
		return 0
	}
	return raw
}

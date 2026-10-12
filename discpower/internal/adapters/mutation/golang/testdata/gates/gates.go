package gates

var limits = map[string]int{}

func init() {
	limits["day"] = 24
}

func init() {
	limits["week"] = 7
}

func Allowed(kind string, used int) bool {
	limit, ok := limits[kind]
	if !ok {
		return false
	}
	return used < limit
}

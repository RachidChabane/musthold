package construct

// Domain is a subject a task is written about.
type Domain struct {
	// Name is the task's name, and its directory's.
	Name string
	// Line is the subject as the author is told it.
	Line string
}

// PassTarget is how many domains must pass acceptance before construction stops.
const PassTarget = 5

// MaxRepairs is how many repair calls one role gets for one domain.
const MaxRepairs = 2

// DomainOrder lists the domains in the order they are processed (PREREG §3.3).
func DomainOrder() []Domain {
	return []Domain{
		{"library-late-fees", "late fees of a library loan by days overdue"},
		{"train-fare", "train fare by distance zones"},
		{"bicycle-rental", "bicycle rental price by duration"},
		{"exam-grade", "exam grade from a score"},
		{"electricity-tariff", "electricity tariff by hour of day"},
		{"loyalty-points", "loyalty points for a purchase amount"},
		{"hotel-night-rate", "hotel night rate by season and length of stay"},
		{"water-bill", "water bill by consumption tiers"},
		{"password-acceptance", "password acceptance by length and character classes"},
		{"luggage-fee", "luggage fee by weight and cabin class"},
	}
}

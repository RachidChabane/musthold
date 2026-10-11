package scanscope

// Queue hands out the mutants of a scan by need: in the start order, a mutant
// that can still move a verdict starts before every mutant that cannot, and a
// mutant that cannot starts only when none that can is left. No mutant is ever
// dropped: a scan that runs every mutant runs the same ones whatever the order
// they finish in.
type Queue struct {
	order   []int
	started []bool
	left    int
}

// NewQueue holds the mutants, by index, in their start order.
func NewQueue(order []int) *Queue {
	started := make([]bool, len(order))
	return &Queue{order: order, started: started, left: len(order)}
}

// Next starts the first mutant of the start order that is needed, else the
// first one left, and reports false when every mutant has started.
func (q *Queue) Next(needed func(index int) bool) (int, bool) {
	if q.left == 0 {
		return 0, false
	}
	first := -1
	for position, index := range q.order {
		if q.started[position] {
			continue
		}
		if needed(index) {
			return q.start(position), true
		}
		if first < 0 {
			first = position
		}
	}
	return q.start(first), true
}

func (q *Queue) start(position int) int {
	q.started[position] = true
	q.left--
	return q.order[position]
}

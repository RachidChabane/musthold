package differential

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
)

// The full resolution of the drawn values. A datetime is drawn to the
// microsecond, the finest step both the Python and the Go probes keep, and every
// number with a step one decimal finer than it is written with, so that inputs
// such as a reversal of a few seconds or a sub-cent amount are drawn, not only
// the whole minutes and cents a spec names.
const (
	// anchorDay is the day every drawn datetime lies on or after.
	anchorDay = "2024-01-01"
	// datetimeLayout writes a drawn datetime without zone, with the fraction of a
	// second only when it is not zero.
	datetimeLayout = "2006-01-02T15:04:05.999999"
	dateLayout     = "2006-01-02"
	// finerDigits is how many decimals finer than its written form a number's
	// finest neighbours are.
	finerDigits = 1
	// halfStepDenominator draws the midpoint between a number and its next step,
	// where rounding changes.
	halfStepDenominator = 2
	hoursPerDay         = 24
)

var (
	// timeOfDayRegexp matches a time of day the way a spec writes it: 22:00,
	// 06:00 or 10:00:30.
	timeOfDayRegexp = regexp.MustCompile(`\b([01]?\d|2[0-3]):([0-5]\d)(?::([0-5]\d))?\b`)
	// durationRegexp matches a duration the way a spec writes it: 15 minutes,
	// 3 hours, a 24-hour period, 2 days.
	durationRegexp = regexp.MustCompile(`\b(\d+)[ -](second|minute|hour|day)s?\b`)
	// quotedRegexp matches a quoted text of a spec.
	quotedRegexp  = regexp.MustCompile(`"([^"\n]{0,24})"`)
	durationUnits = map[string]time.Duration{
		"second": time.Second, "minute": time.Minute, "hour": time.Hour, "day": hoursPerDay * time.Hour,
	}
	// timeSteps are the neighbours of every drawn instant: a microsecond, a
	// second and a minute on each side.
	timeSteps = []time.Duration{time.Microsecond, time.Second, time.Minute}
)

// anchor is midnight of the anchor day, in UTC.
func anchor() time.Time {
	day, err := time.ParseInLocation(dateLayout, anchorDay, time.UTC)
	if err != nil {
		panic("differential: the anchor day does not parse: " + err.Error())
	}
	return day
}

// basicDatetimes are drawn for a datetime parameter whatever the spec and the
// code say: midnight of the anchor day and of the next one, a microsecond after
// midnight, and noon.
func basicDatetimes() []spec.Literal {
	start := anchor()
	return datetimeLiterals(start, start.Add(time.Microsecond), start.Add(hoursPerDay/2*time.Hour), start.Add(hoursPerDay*time.Hour))
}

func datetimeLiterals(instants ...time.Time) []spec.Literal {
	out := make([]spec.Literal, len(instants))
	for index, instant := range instants {
		out[index] = spec.Literal{Type: spec.TypeDatetime, Value: instant.Format(datetimeLayout)}
	}
	return out
}

// withNeighbours lists an instant and its neighbours at every time step.
func withNeighbours(instant time.Time) []time.Time {
	instants := []time.Time{instant}
	for _, step := range timeSteps {
		instants = append(instants, instant.Add(step), instant.Add(-step))
	}
	return instants
}

// WithSpec seeds the pool with the literals of the spec's text: every number with
// its neighbours at full resolution, every time of day on the anchor day and the
// next one, every duration counted from the anchor midnight, each instant with
// its neighbours, and every short quoted text.
func (p Pool) WithSpec(texts []string) Pool {
	seeded := p.clone()
	for _, text := range texts {
		for _, number := range numberRegexp.FindAllString(text, -1) {
			seeded.addNumberWithNeighbours(number)
		}
		for _, match := range quotedRegexp.FindAllStringSubmatch(text, -1) {
			seeded.strings = appendUnique(seeded.strings, spec.Literal{Type: spec.TypeString, Value: match[1]})
		}
		for _, instant := range instantsOf(text) {
			for _, neighbour := range withNeighbours(instant) {
				seeded.datetimes = appendUnique(seeded.datetimes, datetimeLiterals(neighbour)[0])
			}
		}
	}
	return seeded
}

// instantsOf lists the instants a text names: each time of day on the anchor day
// and on the next one, and each duration after the anchor midnight.
func instantsOf(text string) []time.Time {
	start := anchor()
	var instants []time.Time
	for _, match := range timeOfDayRegexp.FindAllStringSubmatch(text, -1) {
		offset := clockOffset(match[1], match[2], match[3])
		instants = append(instants, start.Add(offset), start.Add(hoursPerDay*time.Hour+offset))
	}
	for _, match := range durationRegexp.FindAllStringSubmatch(text, -1) {
		count, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		instants = append(instants, start.Add(time.Duration(count)*durationUnits[strings.ToLower(match[2])]))
	}
	return instants
}

func clockOffset(hours, minutes, seconds string) time.Duration {
	h, _ := strconv.Atoi(hours)
	m, _ := strconv.Atoi(minutes)
	s, _ := strconv.Atoi(seconds)
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second
}

// exampleLayouts are the forms of a datetime an example may be written in.
var exampleLayouts = []string{time.RFC3339Nano, datetimeLayout, dateLayout}

// addDatetimeWithNeighbours adds a datetime an example names with its neighbours
// at every time step; a text that is no datetime adds nothing.
func (p *Pool) addDatetimeWithNeighbours(text string) {
	for _, layout := range exampleLayouts {
		instant, err := time.ParseInLocation(layout, text, time.UTC)
		if err != nil {
			continue
		}
		for _, neighbour := range withNeighbours(instant.UTC()) {
			p.datetimes = appendUnique(p.datetimes, datetimeLiterals(neighbour)[0])
		}
		return
	}
}

func (p Pool) clone() Pool {
	byName := make(map[string][]spec.Literal, len(p.byName))
	for name, values := range p.byName {
		byName[name] = append([]spec.Literal(nil), values...)
	}
	return Pool{
		numbers: append([]spec.Literal(nil), p.numbers...), strings: append([]spec.Literal(nil), p.strings...),
		datetimes: append([]spec.Literal(nil), p.datetimes...), byName: byName,
	}
}

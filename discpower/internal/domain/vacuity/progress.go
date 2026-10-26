package vacuity

import "github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"

// Progress follows a scan while its mutants finish and tells which sampled
// tests no further outcome can move out of their class. A verdict is settled
// in two cases only:
//
//   - the test failed a run on a sentinel: Classify holds it unproven, and a
//     further outcome never takes that failure away;
//   - every sentinel has finished, the test failed a run on none of them, and
//     it killed, every run failing, a mutant that is no sentinel and not
//     identical when compiled, with a mutated line among the lines it ran:
//     that kill proves the mutant and is a change the test caught, and a
//     further outcome never makes the test fail a sentinel.
//
// A test that is not sampled or not green has no verdict to wait for. Every
// other test stays open, whatever its outcomes so far, since one more kill or
// one more survival can still move it. The answers depend only on the set of
// mutants recorded, never on the order they were recorded in.
type Progress struct {
	sampled map[ecosystem.TestID]bool
	green   map[ecosystem.TestID]bool
	// ran holds the lines each sampled test ran, start-up included.
	ran map[ecosystem.TestID]lineSet
	// sentinelsLeft counts the sentinels not recorded yet.
	sentinelsLeft int
	sentinels     map[string]bool
	recorded      map[string]bool
	// sentinelKillers are the tests that failed a run on a sentinel.
	sentinelKillers map[ecosystem.TestID]bool
	// killers are the tests that killed an eligible mutant among the lines
	// they ran.
	killers map[ecosystem.TestID]bool
}

// NewProgress follows a scan of the input's reference runs, sample, coverage
// and mutants; its outcomes are ignored, every mutant starting unrecorded.
func NewProgress(input Input) *Progress {
	progress := &Progress{
		sampled: map[ecosystem.TestID]bool{}, green: map[ecosystem.TestID]bool{}, ran: map[ecosystem.TestID]lineSet{},
		sentinels: map[string]bool{}, recorded: map[string]bool{},
		sentinelKillers: map[ecosystem.TestID]bool{}, killers: map[ecosystem.TestID]bool{},
	}
	for _, test := range input.Sampled {
		progress.sampled[test] = true
		progress.green[test] = Green(input.Reference[test])
		progress.ran[test] = setOf(input.Coverage[test].Lines)
	}
	for _, mutant := range input.Mutants {
		if mutant.Sentinel && !progress.sentinels[mutant.ID] {
			progress.sentinels[mutant.ID] = true
			progress.sentinelsLeft++
		}
	}
	return progress
}

// Record reads the outcomes of one finished mutant, with whether E0 held it
// identical: none for a mutant that did not build. A mutant is recorded once;
// recording it again changes nothing.
func (p *Progress) Record(mutant Mutant, outcomes map[ecosystem.TestID][]Run) {
	if p.recorded[mutant.ID] {
		return
	}
	p.recorded[mutant.ID] = true
	if p.sentinels[mutant.ID] {
		p.sentinelsLeft--
	}
	for test, runs := range outcomes {
		switch {
		case !p.sampled[test]:
		case mutant.Sentinel && failedOnce(runs):
			p.sentinelKillers[test] = true
		case mutant.eligible() && Settle(runs) == SettledKilled && p.ran[test].holdsAny(mutant.Lines):
			p.killers[test] = true
		}
	}
}

// Settled reports whether no further outcome can move the test out of its
// class.
func (p *Progress) Settled(test ecosystem.TestID) bool {
	switch {
	case !p.sampled[test] || !p.green[test] || p.sentinelKillers[test]:
		return true
	}
	return p.killers[test] && p.sentinelsLeft == 0
}

// Open reports whether a sampled test whose verdict can still move ran one of
// the lines: a mutant of those lines can still move a verdict.
func (p *Progress) Open(lines []Line) bool {
	for test := range p.sampled {
		if !p.Settled(test) && p.ran[test].holdsAny(lines) {
			return true
		}
	}
	return false
}

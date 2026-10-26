package requests

import (
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

func TestShadowClassificationsAreObserved(t *testing.T) {
	inside := hostprototest.Epoch.Add(-time.Minute)
	observed := func(t *testing.T, answer func(hostproto.ClassifiedCall) string) (hostproto.ShadowWitness, *classifyFixture) {
		t.Helper()
		f := newClassifyFixture(t)
		item := f.plan(hostproto.ClassifyShadow)
		if _, err := f.store.UpdateEntry(f.ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) {
			e.EmittedAt = hostprototest.Epoch.Add(-time.Hour)
			return e, nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.importWith(hostproto.ClassifyShadow, hostprototest.Always(classifyViolates), f.admission); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.WitnessClassifications(rawLog(t, item, inside, answer)); err != nil {
			t.Fatal(err)
		}
		witness, found := hostproto.ParseShadowWitness(f.lastDetail(hostproto.EventClassifyShadowWitnessed))
		if !found {
			t.Fatalf("no observation in the journal %v", f.journalKinds())
		}
		return witness, f
	}

	t.Run("a shadow classification the raw API log holds is observed with the query sources of its entries and answers nothing [verifies SP-1913~1]", func(t *testing.T) {
		witness, f := observed(t, func(hostproto.ClassifiedCall) string { return classifyViolates })
		if witness.Verdict != hostproto.WitnessRaised || witness.Model != witnessModel || len(witness.QuerySources) != 1 ||
			witness.QuerySources[0] != hostproto.ClassifyQuerySource || witness.LoggedCalls != hostproto.ClassifyCalls {
			t.Fatalf("observation %+v", witness)
		}
		if f.state() != hostproto.StateEmitted || f.journalHas(hostproto.EventWitnessConsumed) || f.journalHas(hostproto.EventClassifyWitnessed) || f.journalHas(hostproto.EventExposed) {
			t.Fatalf("state %s, journal %v", f.state(), f.journalKinds())
		}
		if _, err := f.service.WitnessClassifications(rawLog(t, f.plan(hostproto.ClassifyShadow), inside, func(hostproto.ClassifiedCall) string { return classifyViolates })); err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, kind := range f.journalKinds() {
			if kind == hostproto.EventClassifyShadowWitnessed {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("%d observations of one classification", count)
		}
	})

	t.Run("a shadow classification the log contradicts is observed as such and marks nothing [verifies SP-1913~1]", func(t *testing.T) {
		witness, f := observed(t, func(hostproto.ClassifiedCall) string { return classifyConsistent })
		if witness.Verdict != hostproto.WitnessLabelsContradicted || f.journalHas(hostproto.EventExposed) {
			t.Fatalf("observation %+v, journal %v", witness, f.journalKinds())
		}
	})

	t.Run("a request is planned once: a classified ticket is not planned again [verifies SP-1908~1]", func(t *testing.T) {
		f := newClassifyFixture(t)
		commands := ClassifyCommands{Service: f.service, Kind: format.KindAlignment, Admission: f.admission, Stdout: &strings.Builder{}}
		listing, err := f.service.List(hostproto.EverySession)
		if err != nil {
			t.Fatal(err)
		}
		before, err := commands.unclassified(listing)
		if err != nil || closedItems(before) != 1 {
			t.Fatalf("%d closed requests (%v)", closedItems(before), err)
		}
		if _, err := f.importWith(hostproto.ClassifyShadow, hostprototest.Always(classifyViolates), f.admission); err != nil {
			t.Fatal(err)
		}
		after, err := commands.unclassified(listing)
		if err != nil || closedItems(after) != 0 || len(after.Items) != len(before.Items)-1 {
			t.Fatalf("%d closed requests of %d (%v)", closedItems(after), len(after.Items), err)
		}
	})
}

func closedItems(listing hostproto.Listing) int {
	count := 0
	for _, item := range listing.Items {
		if len(item.View.Closed) > 0 {
			count++
		}
	}
	return count
}

package tariff

import "testing"

func TestPrice(t *testing.T) {
	table := Standard()
	t.Run("near", func(t *testing.T) {
		if got := Price(table, "near"); got != 3 {
			t.Fatalf("near = %d", got)
		}
	})
	t.Run("far", func(t *testing.T) {
		if got := Price(table, "far"); got != 8 {
			t.Fatalf("far = %d", got)
		}
	})
}

func FuzzSurcharge(f *testing.F) {
	f.Add(Price(Standard(), "near"))
	f.Add(Price(Standard(), "far"))
	f.Fuzz(func(t *testing.T, price int) {
		if Surcharge(price) <= price {
			t.Fatalf("Surcharge(%d) = %d", price, Surcharge(price))
		}
	})
}

func TestZones(t *testing.T) {
	var priced []string
	t.Run("near", func(t *testing.T) {
		priced = append(priced, "near")
		if Price(Standard(), "near") == 0 {
			t.Fatal("near is free")
		}
	})
	t.Run("far", func(t *testing.T) {
		priced = append(priced, "far")
		if Price(Standard(), "far") == 0 {
			t.Fatal("far is free")
		}
	})
	if len(priced) != 2 {
		t.Errorf("priced %v", priced)
	}
}

func TestQuotes(t *testing.T) {
	quoted := 0
	t.Cleanup(func() {
		if quoted != 2 {
			t.Errorf("quoted %d zones", quoted)
		}
	})
	t.Run("near", func(t *testing.T) {
		quoted++
		if got := Price(Standard(), "near"); got != 3 {
			t.Fatalf("near = %d", got)
		}
	})
	t.Run("far", func(t *testing.T) {
		quoted++
		if got := Price(Standard(), "far"); got != 8 {
			t.Fatalf("far = %d", got)
		}
	})
}

func TestSamplesAreWarm(t *testing.T) {
	if samples < 1 {
		t.Fatalf("samples = %d", samples)
	}
}

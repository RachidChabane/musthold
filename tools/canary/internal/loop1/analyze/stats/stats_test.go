package stats

import (
	"math"
	"math/big"
	"slices"
	"testing"

	"pgregory.net/rapid"
)

func rat(a, b int64) *big.Rat { return big.NewRat(a, b) }

func equal(t *testing.T, what string, got, want *big.Rat) {
	t.Helper()
	if got.Cmp(want) != 0 {
		t.Errorf("%s = %s, want %s", what, got.RatString(), want.RatString())
	}
}

func TestStratifiedPermutation(t *testing.T) {
	t.Run("one task holds every escape in its control arm [verifies SP-2595~1]", func(t *testing.T) {
		// Five tasks of 3 control and 3 treatment runs. Task 1 escapes in its three
		// control runs, the others never. Under the null, task 1's treatment escapes
		// follow the hypergeometric of 6 runs, 3 escapes, 3 drawn:
		//   P(0) = C(3,0)C(3,3)/C(6,3) = 1/20, P(1) = C(3,1)C(3,2)/20 = 9/20,
		//   P(2) = 9/20, P(3) = 1/20.
		// The other tasks put all their mass on 0, so the sum is task 1's value.
		// Observed treatment escapes: 0. p_less = P(0) = 1/20; p_more = 1.
		// Floor: max(0, 3-3) = 0, so p_min = P(0) = 1/20.
		strata := []Stratum{{3, 3, 3, 0}, {3, 0, 3, 0}, {3, 0, 3, 0}, {3, 0, 3, 0}, {3, 0, 3, 0}}
		got := StratifiedPermutation(strata)
		equal(t, "p_less", got.PLess, rat(1, 20))
		equal(t, "p_more", got.PMore, rat(1, 1))
		equal(t, "p_min", got.PMin, rat(1, 20))
		if !Significant(got.PLess) || got.Observed != 0 || got.Floor != 0 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("two tasks of unequal arms are convolved [verifies SP-2595~1]", func(t *testing.T) {
		// Task 1: 3 control runs with 2 escapes, 3 treatment runs with 0. N = 6, e = 2:
		//   P(0) = C(2,0)C(4,3)/20 = 4/20, P(1) = C(2,1)C(4,2)/20 = 12/20, P(2) = C(2,2)C(4,1)/20 = 4/20.
		// Task 2: 2 control runs with 1 escape, 3 treatment runs with 1. N = 5, e = 2, C(5,3) = 10:
		//   P(0) = C(2,0)C(3,3)/10 = 1/10, P(1) = C(2,1)C(3,2)/10 = 6/10, P(2) = C(2,2)C(3,1)/10 = 3/10.
		// The sum, in fiftieths: P(0) = 4·1/200 = 1/50; P(1) = (4·6 + 12·1)/200 = 9/50;
		//   P(2) = (4·3 + 12·6 + 4·1)/200 = 22/50; P(3) = (12·3 + 4·6)/200 = 15/50; P(4) = 4·3/200 = 3/50.
		// Observed: 0 + 1 = 1. p_less = (1 + 9)/50 = 1/5; p_more = (9 + 22 + 15 + 3)/50 = 49/50.
		// Floor: max(0, 2-3) + max(0, 2-2) = 0, so p_min = 1/50.
		got := StratifiedPermutation([]Stratum{{3, 2, 3, 0}, {2, 1, 3, 1}})
		equal(t, "p_less", got.PLess, rat(1, 5))
		equal(t, "p_more", got.PMore, rat(49, 50))
		equal(t, "p_min", got.PMin, rat(1, 50))
	})
	t.Run("the floor keeps the escapes a small control arm cannot hold [verifies SP-2595~1]", func(t *testing.T) {
		// One task, 2 control and 3 treatment runs, 4 escapes, 2 of them in treatment.
		// N = 5, e = 4, C(5,3) = 10: P(1) = C(4,1)C(1,2)/10 = 0, P(2) = C(4,2)C(1,1)/10 = 6/10,
		//   P(3) = C(4,3)C(1,0)/10 = 4/10. Floor: max(0, 4-2) = 2.
		// Observed 2: p_less = 6/10 = 3/5 = p_min; p_more = 1.
		got := StratifiedPermutation([]Stratum{{2, 2, 3, 2}})
		equal(t, "p_less", got.PLess, rat(3, 5))
		equal(t, "p_min", got.PMin, rat(3, 5))
		equal(t, "p_more", got.PMore, rat(1, 1))
		if got.Floor != 2 {
			t.Fatalf("floor %d", got.Floor)
		}
	})
	t.Run("the null's laws hold on any strata [verifies SP-2595~1]", rapid.MakeCheck(func(t *rapid.T) {
		strata := rapid.SliceOfN(rapid.Custom(func(t *rapid.T) Stratum {
			c := rapid.IntRange(0, 4).Draw(t, "control")
			tr := rapid.IntRange(0, 4).Draw(t, "treatment")
			return Stratum{c, rapid.IntRange(0, c).Draw(t, "ce"), tr, rapid.IntRange(0, tr).Draw(t, "te")}
		}), 1, 7).Draw(t, "strata")
		got := StratifiedPermutation(strata)
		// p_less + p_more = 1 + P(T = observed) >= 1.
		null := Point()
		for _, s := range strata {
			null = null.Convolve(s.Hypergeometric())
		}
		sum := new(big.Rat).Add(got.PLess, got.PMore)
		if sum.Cmp(new(big.Rat).Add(rat(1, 1), null.At(got.Observed))) != 0 || sum.Cmp(rat(1, 1)) < 0 {
			t.Fatalf("p_less + p_more = %s", sum.RatString())
		}
		if got.PMin.Cmp(got.PLess) > 0 {
			t.Fatalf("p_min %s above p_less %s", got.PMin.RatString(), got.PLess.RatString())
		}
		// The order of the tasks changes nothing.
		shuffled := slices.Clone(strata)
		slices.Reverse(shuffled)
		again := StratifiedPermutation(shuffled)
		if again.PLess.Cmp(got.PLess) != 0 || again.PMore.Cmp(got.PMore) != 0 || again.PMin.Cmp(got.PMin) != 0 {
			t.Fatal("the result depends on the order of the tasks")
		}
		// Swapping the arms swaps p_less and p_more.
		swapped := make([]Stratum, len(strata))
		for index, s := range strata {
			swapped[index] = Stratum{s.TreatmentRuns, s.TreatmentEvents, s.ControlRuns, s.ControlEvents}
		}
		mirror := StratifiedPermutation(swapped)
		if mirror.PLess.Cmp(got.PMore) != 0 || mirror.PMore.Cmp(got.PLess) != 0 {
			t.Fatal("swapping the arms does not swap the tails")
		}
	}))
}

func TestStratifiedSum(t *testing.T) {
	t.Run("every label assignment is enumerated and convolved [verifies SP-2600~1]", func(t *testing.T) {
		// Task 1: control kills [1, 2], treatment [3]. One treatment run among three:
		//   sums 1, 2, 3, each 1/3.
		// Task 2: control [0], treatment [2, 2]. Two treatment runs among {0, 2, 2}:
		//   {0,2} = 2 twice, {2,2} = 4 once: P(2) = 2/3, P(4) = 1/3.
		// Observed total: 3 + 4 = 7, the largest sum, reached by 3 + 4 alone: 1/3 · 1/3 = 1/9.
		got := StratifiedSum([]Valued{{Control: []int{1, 2}, Treatment: []int{3}}, {Control: []int{0}, Treatment: []int{2, 2}}})
		if got.Observed != 7 {
			t.Fatalf("observed %d", got.Observed)
		}
		equal(t, "p", got.PMore, rat(1, 9))
	})
	t.Run("equal values give a p of 1 [verifies SP-2600~1]", func(t *testing.T) {
		got := StratifiedSum([]Valued{{Control: []int{2, 2, 2}, Treatment: []int{2, 2, 2}}})
		equal(t, "p", got.PMore, rat(1, 1))
	})
	t.Run("the enumeration is a distribution invariant to the order of the runs [verifies SP-2600~1]", rapid.MakeCheck(func(t *rapid.T) {
		task := Valued{
			Control:   rapid.SliceOfN(rapid.IntRange(0, 5), 0, 4).Draw(t, "control"),
			Treatment: rapid.SliceOfN(rapid.IntRange(0, 5), 0, 4).Draw(t, "treatment"),
		}
		total := new(big.Rat)
		for _, p := range task.SumDistribution() {
			total.Add(total, p)
		}
		if total.Cmp(rat(1, 1)) != 0 {
			t.Fatalf("mass %s", total.RatString())
		}
		reordered := Valued{Control: slices.Clone(task.Control), Treatment: slices.Clone(task.Treatment)}
		slices.Reverse(reordered.Control)
		slices.Reverse(reordered.Treatment)
		if StratifiedSum([]Valued{task}).PMore.Cmp(StratifiedSum([]Valued{reordered}).PMore) != 0 {
			t.Fatal("the result depends on the order of the runs")
		}
	}))
}

func TestBinomialAndSign(t *testing.T) {
	t.Run("the exact one-sided binomial test of a rate above one half [verifies SP-2599~1]", func(t *testing.T) {
		// P(X >= 6 | 6, 1/2) = 1/64; P(X >= 5 | 6) = (6 + 1)/64 = 7/64;
		// P(X >= 0 | n) = 1; P(X >= 7 | 9) = (36 + 9 + 1)/512 = 46/512 = 23/256.
		equal(t, "6 of 6", BinomialUpper(6, 6), rat(1, 64))
		equal(t, "5 of 6", BinomialUpper(5, 6), rat(7, 64))
		equal(t, "0 of 4", BinomialUpper(0, 4), rat(1, 1))
		equal(t, "7 of 9", BinomialUpper(7, 9), rat(23, 256))
	})
	t.Run("the sign test drops ties [verifies SP-2600~1]", func(t *testing.T) {
		// Five positive differences and one zero: P(X >= 5 | 5) = 1/32.
		got := SignTest([]*big.Rat{rat(1, 3), rat(2, 3), rat(1, 1), rat(1, 2), rat(1, 6), rat(0, 1)})
		if got.Favourable != 5 || got.Informative != 5 {
			t.Fatalf("got %+v", got)
		}
		equal(t, "p", got.P, rat(1, 32))
		// Four positive and two negative: P(X >= 4 | 6) = (15 + 6 + 1)/64 = 22/64 = 11/32.
		equal(t, "p", SignTest([]*big.Rat{rat(1, 1), rat(1, 1), rat(1, 1), rat(1, 1), rat(-1, 1), rat(-1, 3)}).P, rat(11, 32))
		equal(t, "no informative task", SignTest(nil).P, rat(1, 1))
	})
}

func TestClopperPearson(t *testing.T) {
	const tolerance = 1e-9
	t.Run("the exact interval matches its known values [verifies SP-2597~1]", func(t *testing.T) {
		// Reference values: the beta quantiles Beta(0.025; k, n-k+1) and
		// Beta(0.975; k+1, n-k), and the closed forms 1 - 0.025^(1/n) at k = 0 and
		// 0.025^(1/n) at k = n.
		cases := []struct {
			k, n      int
			low, high float64
		}{
			{1, 10, 0.0025285785444617848, 0.4450161170281954},
			{5, 10, 0.18708602844739855, 0.8129139715526015},
			{3, 18, 0.035785083121574676, 0.41417749139477406},
			{2, 9, 0.028144973477898197, 0.600093573716312},
			{0, 6, 0, 1 - math.Pow(0.025, 1.0/6)},
			{7, 7, math.Pow(0.025, 1.0/7), 1},
			{0, 3, 0, 1 - math.Pow(0.025, 1.0/3)},
			{3, 3, math.Pow(0.025, 1.0/3), 1},
		}
		for _, c := range cases {
			got := ClopperPearson(c.k, c.n)
			if math.Abs(got.Low-c.low) > tolerance || math.Abs(got.High-c.high) > tolerance {
				t.Errorf("%d/%d: got [%v, %v], want [%v, %v]", c.k, c.n, got.Low, got.High, c.low, c.high)
			}
		}
		if got := ClopperPearson(0, 0); got.Low != 0 || got.High != 1 {
			t.Errorf("0/0: %+v", got)
		}
	})
	t.Run("the interval holds its estimate [verifies SP-2597~1]", rapid.MakeCheck(func(t *rapid.T) {
		n := rapid.IntRange(1, 40).Draw(t, "n")
		k := rapid.IntRange(0, n).Draw(t, "k")
		got := ClopperPearson(k, n)
		estimate := float64(k) / float64(n)
		if got.Low > estimate+tolerance || got.High < estimate-tolerance || got.Low < 0 || got.High > 1 {
			t.Fatalf("%d/%d: %+v", k, n, got)
		}
	}))
}

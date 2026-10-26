package jvm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

const (
	// maxNonCompilingShare is the share of operator mutants that may fail to
	// compile before the engine is considered to propose too many broken ones.
	maxNonCompilingShare = 0.15
	compileTimeout       = 2 * time.Minute
	compiledFileName     = "Pricing.java"
	javacBinary          = "javac"
	niceBinary           = "nice"
	niceLevel            = "10"
)

// pricingSource is a dependency-free Java class that exercises every family.
const pricingSource = `import java.math.BigDecimal;
import java.time.Duration;

class Pricing {
    static final int GRACE_MINUTES = 15;
    static final BigDecimal BASE = new BigDecimal("4.50");

    static int hours(int minutes) {
        if (minutes <= GRACE_MINUTES) {
            return 0;
        }
        int billed = minutes - GRACE_MINUTES;
        return (billed + 59) / 60;
    }

    static BigDecimal fee(int minutes, boolean member) {
        int h = hours(minutes);
        BigDecimal gross = BASE.multiply(BigDecimal.valueOf(h));
        if (member && h > 2) {
            return gross.min(new BigDecimal("20.00"));
        }
        return gross;
    }

    static long window(long a) {
        Duration d = Duration.ofMinutes(30);
        return Math.max(a, d.toMinutes());
    }

    static void validate(int minutes) {
        try {
            if (minutes < 0) {
                throw new IllegalStateException("negative");
            }
        } catch (RuntimeException e) {
            throw new IllegalArgumentException("invalid");
        }
    }

    static char grade(int minutes) {
        if (minutes > 60) {
            return 'a';
        } else {
            return 'b';
        }
    }

    static class Money {
        private final int cents;

        Money(int cents) {
            this.cents = cents;
        }

        int cents() {
            return cents;
        }
    }

    static Money tip() {
        return new Money(5);
    }

    static class Ledger {
        private BigDecimal total = BigDecimal.ZERO;

        void add(BigDecimal amount) {
            if (amount.signum() > 0) {
                total = total.add(amount);
            }
        }

        boolean empty() {
            boolean zero = false;
            return zero || total.signum() == 0;
        }
    }

    static String label(int minutes) {
        String text = "t" + minutes;
        boolean extended = minutes > 120 || minutes < 0;
        return extended ? text + "!" : text;
    }
}
`

// compiles runs javac on a source and reports whether it accepted it.
func compiles(t *testing.T, source []byte) bool {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, compiledFileName)
	if err := os.WriteFile(file, source, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), compileTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, niceBinary, "-n", niceLevel, javacBinary, "-proc:none",
		"-J-XX:TieredStopAtLevel=1", "-J-XX:+UseSerialGC", "-d", dir, file)
	return command.Run() == nil
}

func TestMutantsCompile(t *testing.T) {
	if _, err := exec.LookPath(javacBinary); err != nil {
		t.Skipf("javac is not on PATH: %v", err)
	}
	engine := New()
	source := []byte(pricingSource)
	file := sourceFile("src/Pricing.java", pricingSource)

	t.Run("the original compiles [verifies SP-1106~1]", func(t *testing.T) {
		if !compiles(t, source) {
			t.Fatal("the corpus does not compile")
		}
	})

	t.Run("javac rejects a broken source, so the gate discriminates [verifies SP-1106~1]", func(t *testing.T) {
		if compiles(t, []byte("class Pricing { int f() { } }")) {
			t.Fatal("javac accepted a method without a return")
		}
	})

	t.Run("every sentinel compiles [verifies SP-1106~1]", func(t *testing.T) {
		sentinels, err := engine.SentinelCandidates(file, operator.LocalSeed)
		if err != nil || len(sentinels) == 0 {
			t.Fatalf("sentinels %d err %v", len(sentinels), err)
		}
		for _, s := range sentinels {
			spliced, err := splice(file.Path, source, s.Edit)
			if err != nil {
				t.Fatal(err)
			}
			if !compiles(t, spliced) {
				t.Errorf("%s sentinel of %s does not compile:\n%s", s.Kind, s.Edit.Qualname, s.Edit.NewSource)
			}
		}
	})

	t.Run("the operator mutants that parse mostly compile [verifies SP-1106~1]", func(t *testing.T) {
		candidates, err := engine.Candidates(file)
		if err != nil {
			t.Fatal(err)
		}
		kept, _, err := operator.Screen(candidates, engine)
		if err != nil || len(kept) == 0 {
			t.Fatalf("kept %d err %v", len(kept), err)
		}
		offered := map[operator.Family]bool{}
		for _, m := range kept {
			offered[m.Family] = true
		}
		for _, family := range engine.Families() {
			if !offered[family] {
				t.Errorf("the corpus yields no %s mutant", family)
			}
		}
		failed := map[operator.Family]int{}
		total := 0
		for _, m := range kept {
			edit := m.EditSet.Edits[0]
			spliced, err := splice(file.Path, source, edit)
			if err != nil {
				t.Fatal(err)
			}
			if !compiles(t, spliced) {
				failed[m.Family]++
				total++
				t.Logf("does not compile (%s, %s): %s", m.Family, edit.Qualname, edit.NewSource)
			}
		}
		share := float64(total) / float64(len(kept))
		t.Logf("%d of %d operator mutants do not compile (%.1f percent): %v", total, len(kept), 100*share, failed)
		if share > maxNonCompilingShare {
			t.Fatalf("%.1f percent of the operator mutants do not compile, above %.0f percent", 100*share, 100*maxNonCompilingShare)
		}
	})
}

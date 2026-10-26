package memguard_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/s1relay/internal/adapters/memguard"
)

func output(freePercent int) []byte {
	return []byte(fmt.Sprintf("The system has 25769803776 (1572864 pages with a page size of 16384).\n\nSystem-wide memory free percentage: %d%%\n", freePercent))
}

func reading(body []byte, err error) memguard.Reader {
	return func(context.Context) ([]byte, error) { return body, err }
}

func TestParseFreePercent(t *testing.T) {
	t.Run("the free percentage is read from the memory_pressure report [verifies SP-612~1]", func(t *testing.T) {
		got, err := memguard.ParseFreePercent(output(83))
		if err != nil || got != 83 {
			t.Fatalf("ParseFreePercent = %d, %v", got, err)
		}
	})

	t.Run("a report without the line or with an impossible value is an error [verifies SP-612~1]", func(t *testing.T) {
		for _, body := range []string{"", "memory is fine", "System-wide memory free percentage: 140%", "System-wide memory free percentage: 99999999999%"} {
			if _, err := memguard.ParseFreePercent([]byte(body)); err == nil {
				t.Errorf("%q parsed", body)
			}
		}
	})
}

func TestCheck(t *testing.T) {
	darwin := func(read memguard.Reader) memguard.Guard {
		return memguard.Guard{OS: memguard.OSDarwin, MinFreePercent: memguard.DefaultMinFreePercent, Read: read}
	}

	t.Run("the batch is allowed at the threshold and above [verifies SP-612~1]", func(t *testing.T) {
		for _, free := range []int{40, 41, 83, 100} {
			verdict, err := darwin(reading(output(free), nil)).Check(context.Background())
			if err != nil || !verdict.Allowed || verdict.Skipped {
				t.Errorf("%d%% free: verdict %+v, %v", free, verdict, err)
			}
		}
	})

	t.Run("the batch is refused below 40 percent free with the figures in the reason [verifies SP-612~1]", func(t *testing.T) {
		for _, free := range []int{0, 12, 39} {
			verdict, err := darwin(reading(output(free), nil)).Check(context.Background())
			if err != nil || verdict.Allowed || verdict.Skipped || !strings.Contains(verdict.Reason, fmt.Sprintf("%d%%", free)) || !strings.Contains(verdict.Reason, "40%") {
				t.Errorf("%d%% free: verdict %+v, %v", free, verdict, err)
			}
		}
	})

	t.Run("the threshold is the one given, which the environment variable of the repository sets [verifies SP-612~1]", func(t *testing.T) {
		guard := memguard.Guard{OS: memguard.OSDarwin, MinFreePercent: 60, Read: reading(output(50), nil)}
		if verdict, _ := guard.Check(context.Background()); verdict.Allowed {
			t.Fatalf("50%% free passed a 60%% rule: %+v", verdict)
		}
		if memguard.MinFreeVariable != "MEM_GUARD_MIN_FREE_PCT" || memguard.DefaultMinFreePercent != 40 {
			t.Fatalf("variable %q default %d", memguard.MinFreeVariable, memguard.DefaultMinFreePercent)
		}
	})

	t.Run("a report that cannot be obtained or read fails closed [verifies SP-612~1]", func(t *testing.T) {
		if _, err := darwin(reading(nil, errors.New("not found"))).Check(context.Background()); err == nil {
			t.Error("a failing command was allowed")
		}
		if _, err := darwin(reading([]byte("garbage"), nil)).Check(context.Background()); err == nil {
			t.Error("an unreadable report was allowed")
		}
	})

	t.Run("on another operating system the rule is skipped with a reason and reads nothing [verifies SP-612~1]", func(t *testing.T) {
		for _, goos := range []string{"linux", "windows", "freebsd"} {
			guard := memguard.Guard{OS: goos, MinFreePercent: 40, Read: func(context.Context) ([]byte, error) { t.Fatal("memory_pressure was run"); return nil, nil }}
			verdict, err := guard.Check(context.Background())
			if err != nil || !verdict.Skipped || !strings.Contains(verdict.Reason, goos) {
				t.Errorf("%s: verdict %+v, %v", goos, verdict, err)
			}
		}
	})
}

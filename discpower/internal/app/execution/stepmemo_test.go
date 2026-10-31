package execution_test

import (
	"context"
	"errors"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/app/execution"
	"github.com/RachidChabane/musthold/format/hostproto"
)

const verifiesStepMemo = " [verifies SP-2671~1]"

type mapMemo map[string][]byte

func (m mapMemo) Get(key string) ([]byte, bool, error) {
	value, found := m[key]
	return value, found, nil
}

func (m mapMemo) Put(key string, value []byte) error {
	m[key] = value
	return nil
}

func TestStepMemo(t *testing.T) {
	t.Run("a step stores and counts facts while it runs and stores none once its budget ran out"+verifiesStepMemo, func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(context.Background())
		inner := mapMemo{}
		memo := execution.NewStepMemo(ctx, inner)
		if err := memo.Put("before", []byte("1")); err != nil || memo.Stored() != 1 {
			t.Fatalf("a fact while the step runs: %v, stored %d", err, memo.Stored())
		}
		cancel(hostproto.ErrStepSpent)
		if err := memo.Put("after", []byte("2")); !errors.Is(err, hostproto.ErrStepSpent) {
			t.Fatalf("a fact after the cut: %v", err)
		}
		if _, found := inner["after"]; found || memo.Stored() != 1 {
			t.Fatalf("the cut fact was kept: %v, stored %d", inner, memo.Stored())
		}
		if value, found, _ := memo.Get("before"); !found || string(value) != "1" {
			t.Fatal("a fact stored before the cut is lost")
		}
	})
}

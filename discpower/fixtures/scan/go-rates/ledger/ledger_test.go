package ledger

import "testing"

func TestPostSumsTheEntries(t *testing.T) {
	if got := Post([]int{120, 80, -20}); got != 180 {
		t.Fatalf("Post = %d, want 180", got)
	}
}

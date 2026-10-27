//go:build linux

package process

import (
	"os"
	"path/filepath"
	"strconv"
)

const (
	// procDirectory holds a directory per process.
	procDirectory = "/proc"
	// oomScoreAdjustFile is where Linux reads how much a process volunteers
	// to the out-of-memory killer; any process may raise its children's.
	oomScoreAdjustFile = "oom_score_adj"
	// firstVictimScore makes a process the first the killer chooses.
	firstVictimScore = "1000"
	oomScoreFileMode = 0o644
)

// preferAsMemoryVictim makes the started process, and the processes it starts
// afterwards, the out-of-memory killer's first choice; a process the system
// does not let volunteer keeps its score.
func preferAsMemoryVictim(pid int) {
	_ = os.WriteFile(filepath.Join(procDirectory, strconv.Itoa(pid), oomScoreAdjustFile), []byte(firstVictimScore), oomScoreFileMode)
}

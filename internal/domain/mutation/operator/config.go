package operator

// Defaults of operator mutant generation.
const (
	// DefaultKOp is the default cap on operator mutants per criterion.
	DefaultKOp = 400
	// SentinelsPerFile is the number of sentinels drawn per footprint file.
	SentinelsPerFile = 2
	// LocalSeed is the seed used for local runs; CI draws a fresh seed per run.
	LocalSeed Seed = 0x5eed5eed
)

// Seed drives every random choice of generation.
type Seed uint64

// Config parameterizes a generation run.
type Config struct {
	// KOp caps the operator mutants kept after sampling; sentinels do not count.
	KOp  int
	Seed Seed
}

// DefaultConfig is the configuration of a local run.
func DefaultConfig() Config { return Config{KOp: DefaultKOp, Seed: LocalSeed} }

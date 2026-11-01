package task

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
)

// Config is loop-2.json: loop-1's run configuration with the seed of the
// domains' draw.
type Config struct {
	config.Config
	// DrawSeed seeds the permutation of the domains.
	DrawSeed uint64 `json:"draw_seed"`
}

// ErrConfig is returned for a loop-2.json that cannot be used.
var ErrConfig = errors.New("loop-2.json is invalid")

// ParseConfig reads loop-2.json, refusing unknown members, requiring a draw seed
// and, once it lists a task, validating loop-1's members by loop-1's rules.
func ParseConfig(data []byte) (Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var parsed Config
	if err := decoder.Decode(&parsed); err != nil {
		return Config{}, fmt.Errorf("%w: %v", ErrConfig, err)
	}
	if decoder.More() {
		return Config{}, fmt.Errorf("%w: more than one document", ErrConfig)
	}
	if parsed.DrawSeed == 0 {
		return Config{}, fmt.Errorf("%w: draw_seed: required", ErrConfig)
	}
	if len(parsed.Tasks) == 0 {
		return parsed, nil
	}
	base, err := json.Marshal(parsed.Config)
	if err != nil {
		return Config{}, err
	}
	if _, err := config.Parse(base); err != nil {
		return Config{}, fmt.Errorf("%w: %v", ErrConfig, err)
	}
	return parsed, nil
}

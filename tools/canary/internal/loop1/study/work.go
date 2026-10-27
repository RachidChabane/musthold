package study

import (
	"os"
	"path/filepath"
)

// WorkEnv names the environment variable that holds the work directory, and
// DefaultWorkName the directory under the home directory used when it is unset
// (PREREG §1).
const (
	WorkEnv         = "LOOP1_WORK"
	DefaultWorkName = ".musthold-loop-1"
)

// DefaultWork is $LOOP1_WORK, or ~/.musthold-loop-1.
func DefaultWork() string {
	if work := os.Getenv(WorkEnv); work != "" {
		return work
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultWorkName
	}
	return filepath.Join(home, DefaultWorkName)
}

// ProductEnv names the environment variable that holds the product directory, and
// DefaultProductName the directory under the home directory used when it is unset.
const (
	ProductEnv         = "LOOP1_PRODUCT"
	DefaultProductName = ".musthold-loop-1-product"
)

// DefaultProduct is $LOOP1_PRODUCT, or ~/.musthold-loop-1-product.
func DefaultProduct() string {
	if dir := os.Getenv(ProductEnv); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultProductName
	}
	return filepath.Join(home, DefaultProductName)
}

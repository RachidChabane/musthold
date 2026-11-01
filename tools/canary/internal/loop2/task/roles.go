package task

import "github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"

// The roles of loop-2's construction beside loop-1's author and oracle.
const (
	// RoleVariants writes the violations and the refactors from the spec and
	// the reference.
	RoleVariants records.Role = "variants"
	// RoleSecondOracle writes the second, independent oracle from the spec and
	// the interface alone.
	RoleSecondOracle records.Role = "second-oracle"
	// RoleCiter cites the spec sentence behind each assertion of the oracle.
	RoleCiter records.Role = "citer"
)

// Package operator is the deterministic, model-free part of mutant generation:
// operator mutants over typed sites and families, semantic no-op sentinels,
// F1 normalization and deduplication, and seeded stratified sampling to K_op.
package operator

import "slices"

// Family is the kind of change an operator makes. The values are the family
// names recorded with a mutant.
type Family string

// Operator families.
const (
	FamilyRelational          Family = "relational"
	FamilyArithmetic          Family = "arithmetic"
	FamilyNumericConstant     Family = "numeric_constant"
	FamilyBooleanLogical      Family = "boolean_logical"
	FamilyReturnValue         Family = "return_value"
	FamilyModuleConstant      Family = "module_constant"
	FamilyConstructorArgument Family = "constructor_argument"
	FamilyDecimalString       Family = "decimal_string"
	FamilyMinMax              Family = "min_max"
	FamilyAndOr               Family = "and_or"
	FamilyBooleanLiteral      Family = "boolean_literal"
	FamilyExceptionClass      Family = "exception_class"
)

var allFamilies = []Family{
	FamilyRelational, FamilyArithmetic, FamilyNumericConstant, FamilyBooleanLogical,
	FamilyReturnValue, FamilyModuleConstant, FamilyConstructorArgument,
	FamilyDecimalString, FamilyMinMax, FamilyAndOr, FamilyBooleanLiteral, FamilyExceptionClass,
}

// Families lists every family in a stable order.
func Families() []Family { return slices.Clone(allFamilies) }

// Valid reports whether f is a known family.
func (f Family) Valid() bool { return slices.Contains(allFamilies, f) }

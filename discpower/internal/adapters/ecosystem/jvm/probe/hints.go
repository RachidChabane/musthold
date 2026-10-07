package probe

import "github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"

// hintsByType is the type hint of the declared types the probe program converts
// literals to, Java and Kotlin spellings together.
var hintsByType = map[string]differential.Hint{
	"int": differential.HintInteger, "long": differential.HintInteger, "short": differential.HintInteger,
	"byte": differential.HintInteger, "Integer": differential.HintInteger, "Long": differential.HintInteger,
	"Short": differential.HintInteger, "Byte": differential.HintInteger, "BigInteger": differential.HintInteger,
	"Int": differential.HintInteger,

	"double": differential.HintDecimal, "float": differential.HintDecimal, "Double": differential.HintDecimal,
	"Float": differential.HintDecimal, "BigDecimal": differential.HintDecimal,

	"String": differential.HintString, "char": differential.HintString, "Character": differential.HintString,
	"Char": differential.HintString, "CharSequence": differential.HintString,

	"boolean": differential.HintBoolean, "Boolean": differential.HintBoolean,

	"LocalDate": differential.HintDatetime, "LocalDateTime": differential.HintDatetime,
	"Instant": differential.HintDatetime, "OffsetDateTime": differential.HintDatetime,
	"ZonedDateTime": differential.HintDatetime, "LocalTime": differential.HintDatetime,
	"Date": differential.HintDatetime,
}

// HintOf gives the hint of a declared type; a type the probe program cannot draw
// literals for is unknown.
func HintOf(declared string) differential.Hint {
	if hint, known := hintsByType[declared]; known {
		return hint
	}
	return differential.HintUnknown
}

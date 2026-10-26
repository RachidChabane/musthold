package probe

import (
	"strconv"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
)

const (
	fieldSeparator  = "\t"
	lineSeparator   = "\n"
	callRecord      = "CALL"
	missingType     = "missing"
	escapeCharacter = `\`
)

// escapes maps the characters a field may not hold to their escaped spelling.
var escapes = strings.NewReplacer(
	escapeCharacter, escapeCharacter+escapeCharacter,
	"\t", escapeCharacter+"t",
	"\n", escapeCharacter+"n",
	"\r", escapeCharacter+"r",
)

// Call is one call of a method with its arguments in parameter order.
type Call struct {
	// Class is the binary name of the class that declares the method.
	Class  string
	Method string
	// Args holds one literal per parameter; a nil entry is an argument the call
	// did not supply.
	Args []*spec.Literal
}

// Encode writes the request file of the calls: one line per call, tab separated.
func Encode(calls []Call) []byte {
	var out strings.Builder
	for _, call := range calls {
		fields := []string{callRecord, call.Class, call.Method, strconv.Itoa(len(call.Args))}
		for _, arg := range call.Args {
			if arg == nil {
				fields = append(fields, missingType, "")
				continue
			}
			fields = append(fields, string(arg.Type), escapes.Replace(arg.Value))
		}
		out.WriteString(strings.Join(fields, fieldSeparator))
		out.WriteString(lineSeparator)
	}
	return []byte(out.String())
}

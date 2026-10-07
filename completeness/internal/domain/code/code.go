// Package code holds the units of source the module reasons about: the functions,
// methods and module-level assignments of a source file, each with the text a
// host is shown, a normal form that ignores formatting, comments and docstrings,
// and the names it mentions. A per-ecosystem indexer produces them; nothing else
// in the module knows a language.
package code

// Kind tells what a unit is.
type Kind string

// Kinds of unit.
const (
	// KindFunction is a function or a method.
	KindFunction Kind = "function"
	// KindAssignment is a module-level or package-level variable or constant.
	KindAssignment Kind = "assignment"
)

// Unit is one function, method or module-level assignment of a source file.
type Unit struct {
	// File is the path of the file relative to the project root, with `/`
	// separators.
	File string
	// Qualname names the unit inside its file, such as `Cart.total` or `total`.
	Qualname string
	// Name is the last segment of the qualname.
	Name string
	// Owner is the class or receiver type of a method, empty otherwise.
	Owner string
	Kind  Kind
	// Normalized is the unit's code in a form that ignores formatting, comments
	// and docstrings; two units with the same normal form behave alike.
	Normalized string
	// Text is the code a host is shown.
	Text      string
	StartLine int
	EndLine   int
	// Mentions lists the names the unit refers to, sorted and without repeats.
	Mentions []string
}

// Key identifies the unit inside a project.
func (u Unit) Key() string { return u.File + "\x00" + u.Qualname }

// File is a source file and its units.
type File struct {
	Path string
	// Test is true for a file that holds tests.
	Test  bool
	Units []Unit
}

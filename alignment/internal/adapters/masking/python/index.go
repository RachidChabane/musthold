package python

import (
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// classnameSeparator joins the parts of a dotted module path.
const classnameSeparator = "."

// declaration is a module-level name: a function, a class or a variable.
type declaration struct {
	name string
	// statement is the whole top-level statement, decorators included.
	statement *gotreesitter.Node
	// definition is the function or class the statement defines, nil for a
	// variable.
	definition *gotreesitter.Node
	order      int
}

// method is a function declared in the body of a class.
type method struct {
	name      string
	statement *gotreesitter.Node
	order     int
}

// classScope is a top-level class and its methods in source order.
type classScope struct {
	name    string
	methods []method
}

func (c classScope) method(name string) (method, bool) {
	for _, candidate := range c.methods {
		if candidate.name == name {
			return candidate, true
		}
	}
	return method{}, false
}

// fileIndex is a parsed file with its module-level names and classes.
type fileIndex struct {
	*parsed
	declarations map[string]declaration
	classes      []classScope
}

func indexFile(source []byte) (*fileIndex, error) {
	parsedFile, err := parseSource(source)
	if err != nil {
		return nil, err
	}
	index := &fileIndex{parsed: parsedFile, declarations: map[string]declaration{}}
	for order, statement := range parsedFile.statementsOf(parsedFile.root()) {
		index.record(statement, order)
	}
	return index, nil
}

// record registers the name a top-level statement binds. A name bound twice
// keeps its last binding, as Python does.
func (x *fileIndex) record(statement *gotreesitter.Node, order int) {
	if definition := x.definitionOf(statement); definition != nil {
		name := x.text(x.child(definition, fieldName))
		x.declarations[name] = declaration{name: name, statement: statement, definition: definition, order: order}
		if x.typeOf(definition) == typeClass {
			x.classes = append(x.classes, classScope{name: name, methods: x.methodsOf(definition)})
		}
		return
	}
	if name, ok := x.variableName(statement); ok {
		x.declarations[name] = declaration{name: name, statement: statement, order: order}
	}
}

// variableName is the name of a `name = value` or `name: type = value`
// statement.
func (x *fileIndex) variableName(statement *gotreesitter.Node) (string, bool) {
	assignment := x.bare(statement)
	if x.typeOf(assignment) != typeAssignment {
		return "", false
	}
	target := x.child(assignment, fieldLeft)
	if x.typeOf(target) != typeIdentifier {
		return "", false
	}
	return x.text(target), true
}

func (x *fileIndex) methodsOf(class *gotreesitter.Node) []method {
	body := x.child(class, fieldBody)
	if body == nil {
		return nil
	}
	var methods []method
	for order, statement := range x.statementsOf(body) {
		definition := x.definitionOf(statement)
		if definition != nil && x.typeOf(definition) == typeFunction {
			methods = append(methods, method{name: x.text(x.child(definition, fieldName)), statement: statement, order: order})
		}
	}
	return methods
}

// lastSegment is the part of a dotted path after its last dot.
func lastSegment(dotted string) string {
	return dotted[strings.LastIndex(dotted, classnameSeparator)+1:]
}

// findClassOf returns the class that declares the method. The classname of a
// report names the class when it ends with one; any other class declaring the
// name serves otherwise.
func (x *fileIndex) findClassOf(classname, name string) (classScope, method, bool) {
	hint := lastSegment(classname)
	for _, class := range x.classes {
		if found, ok := class.method(name); ok && class.name == hint {
			return class, found, true
		}
	}
	return classScope{}, method{}, false
}

// findFunction returns the module-level function with the name.
func (x *fileIndex) findFunction(name string) (declaration, bool) {
	found, ok := x.declarations[name]
	if !ok || found.definition == nil || x.typeOf(found.definition) != typeFunction {
		return declaration{}, false
	}
	return found, true
}

// findAnyMethod returns the first class that declares a method with the name.
func (x *fileIndex) findAnyMethod(name string) (classScope, method, bool) {
	for _, class := range x.classes {
		if found, ok := class.method(name); ok {
			return class, found, true
		}
	}
	return classScope{}, method{}, false
}

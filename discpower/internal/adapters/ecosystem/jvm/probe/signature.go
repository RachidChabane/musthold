package probe

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Errors of the signature reading.
var (
	// ErrUnsupportedSource is returned for a file that is neither Java nor Kotlin.
	ErrUnsupportedSource = errors.New("not a Java or Kotlin source")
	// ErrNoMethod is returned when the source declares no method of the name in
	// the class asked for.
	ErrNoMethod = errors.New("no such method")
	// ErrOverloaded is returned when several methods of the class have the name,
	// which an operation target cannot tell apart.
	ErrOverloaded = errors.New("overloaded method")
)

// Parameter is one parameter of a method as its source declares it.
type Parameter struct {
	Name string
	// Type is the declared type with its package, generic arguments and
	// nullability left out.
	Type string
}

// Source file extensions.
const (
	JavaExtension   = ".java"
	KotlinExtension = ".kt"
)

// Node types and fields of the Java grammar.
const (
	javaMethod       = "method_declaration"
	javaParameter    = "formal_parameter"
	javaSpread       = "spread_parameter"
	javaDeclarator   = "variable_declarator"
	javaTypeField    = "type"
	javaNameField    = "name"
	javaBodyField    = "body"
	javaParamsField  = "parameters"
	spreadSuffix     = "[]"
	genericOpen      = "<"
	nullableMark     = "?"
	packageSeparator = "."
	kotlinFunction   = "function_declaration"
	kotlinParameters = "function_value_parameters"
	kotlinParameter  = "parameter"
	kotlinName       = "simple_identifier"
	kotlinTypeName   = "type_identifier"
	kotlinClassBody  = "class_body"
)

var javaContainers = map[string]bool{
	"class_declaration": true, "interface_declaration": true, "enum_declaration": true, "record_declaration": true,
}

var kotlinContainers = map[string]bool{"class_declaration": true, "object_declaration": true}

// Parameters reads the parameters of the method of the class named by the chain
// of nested class names; an empty chain names the top level of a Kotlin file.
func Parameters(filePath string, source []byte, classes []string, method string) ([]Parameter, error) {
	var language *gotreesitter.Language
	var reader sourceReader
	switch path.Ext(filePath) {
	case JavaExtension:
		language, reader = grammars.JavaLanguage(), javaReader{}
	case KotlinExtension:
		language, reader = grammars.KotlinLanguage(), kotlinReader{}
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedSource, filePath)
	}
	tree, err := gotreesitter.NewParser(language).Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", filePath, err)
	}
	defer tree.Release()
	scope := &reading{lang: language, source: source}
	return reader.parameters(scope, tree.RootNode(), classes, method)
}

// reading is a parsed file being read.
type reading struct {
	lang   *gotreesitter.Language
	source []byte
}

func (r *reading) typeOf(n *gotreesitter.Node) string { return n.Type(r.lang) }

func (r *reading) text(n *gotreesitter.Node) string { return n.Text(r.source) }

func (r *reading) field(n *gotreesitter.Node, name string) *gotreesitter.Node {
	return n.ChildByFieldName(name, r.lang)
}

// sourceReader reads the methods of one language.
type sourceReader interface {
	parameters(r *reading, root *gotreesitter.Node, classes []string, method string) ([]Parameter, error)
}

func exactlyOne(found [][]Parameter, method string) ([]Parameter, error) {
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("%w: %s", ErrNoMethod, method)
	case 1:
		return found[0], nil
	}
	return nil, fmt.Errorf("%w: %s", ErrOverloaded, method)
}

type javaReader struct{}

func (javaReader) parameters(r *reading, root *gotreesitter.Node, classes []string, method string) ([]Parameter, error) {
	container := root
	for _, name := range classes {
		next := javaDeclaration(r, container, name)
		if next == nil {
			return nil, fmt.Errorf("%w: class %s", ErrNoMethod, name)
		}
		container = next
	}
	body := r.field(container, javaBodyField)
	if body == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoMethod, method)
	}
	var found [][]Parameter
	for i := 0; i < body.NamedChildCount(); i++ {
		member := body.NamedChild(i)
		if r.typeOf(member) == javaMethod && r.text(r.field(member, javaNameField)) == method {
			found = append(found, javaParameters(r, member))
		}
	}
	return exactlyOne(found, method)
}

// javaDeclaration finds the class, interface, enum or record named name among the
// children of container, looking into the body of a container that is a type.
func javaDeclaration(r *reading, container *gotreesitter.Node, name string) *gotreesitter.Node {
	scope := container
	if body := r.field(container, javaBodyField); body != nil {
		scope = body
	}
	for i := 0; i < scope.NamedChildCount(); i++ {
		child := scope.NamedChild(i)
		if javaContainers[r.typeOf(child)] && r.text(r.field(child, javaNameField)) == name {
			return child
		}
	}
	return nil
}

func javaParameters(r *reading, method *gotreesitter.Node) []Parameter {
	list := r.field(method, javaParamsField)
	var parameters []Parameter
	for i := 0; list != nil && i < list.NamedChildCount(); i++ {
		parameter := list.NamedChild(i)
		switch r.typeOf(parameter) {
		case javaParameter:
			parameters = append(parameters, Parameter{
				Name: r.text(r.field(parameter, javaNameField)),
				Type: simpleType(r.text(r.field(parameter, javaTypeField))),
			})
		case javaSpread:
			parameters = append(parameters, spreadParameter(r, parameter))
		}
	}
	return parameters
}

func spreadParameter(r *reading, parameter *gotreesitter.Node) Parameter {
	var spread Parameter
	for i := 0; i < parameter.NamedChildCount(); i++ {
		child := parameter.NamedChild(i)
		if r.typeOf(child) == javaDeclarator {
			spread.Name = r.text(r.field(child, javaNameField))
		} else if spread.Type == "" {
			spread.Type = simpleType(r.text(child)) + spreadSuffix
		}
	}
	return spread
}

type kotlinReader struct{}

func (kotlinReader) parameters(r *reading, root *gotreesitter.Node, classes []string, method string) ([]Parameter, error) {
	scope := root
	for _, name := range classes {
		next := kotlinDeclaration(r, scope, name)
		if next == nil {
			return nil, fmt.Errorf("%w: class %s", ErrNoMethod, name)
		}
		scope = kotlinBody(r, next)
	}
	var found [][]Parameter
	for i := 0; scope != nil && i < scope.NamedChildCount(); i++ {
		member := scope.NamedChild(i)
		if r.typeOf(member) == kotlinFunction && kotlinFunctionName(r, member) == method {
			found = append(found, kotlinParametersOf(r, member))
		}
	}
	return exactlyOne(found, method)
}

func kotlinBody(r *reading, declaration *gotreesitter.Node) *gotreesitter.Node {
	for i := 0; i < declaration.NamedChildCount(); i++ {
		if child := declaration.NamedChild(i); r.typeOf(child) == kotlinClassBody {
			return child
		}
	}
	return nil
}

func kotlinDeclaration(r *reading, scope *gotreesitter.Node, name string) *gotreesitter.Node {
	for i := 0; scope != nil && i < scope.NamedChildCount(); i++ {
		child := scope.NamedChild(i)
		if !kotlinContainers[r.typeOf(child)] {
			continue
		}
		for j := 0; j < child.NamedChildCount(); j++ {
			if identifier := child.NamedChild(j); r.typeOf(identifier) == kotlinTypeName && r.text(identifier) == name {
				return child
			}
		}
	}
	return nil
}

func kotlinFunctionName(r *reading, function *gotreesitter.Node) string {
	for i := 0; i < function.NamedChildCount(); i++ {
		if child := function.NamedChild(i); r.typeOf(child) == kotlinName {
			return r.text(child)
		}
	}
	return ""
}

func kotlinParametersOf(r *reading, function *gotreesitter.Node) []Parameter {
	var parameters []Parameter
	for i := 0; i < function.NamedChildCount(); i++ {
		list := function.NamedChild(i)
		if r.typeOf(list) != kotlinParameters {
			continue
		}
		for j := 0; j < list.NamedChildCount(); j++ {
			if parameter := list.NamedChild(j); r.typeOf(parameter) == kotlinParameter {
				parameters = append(parameters, kotlinParameterOf(r, parameter))
			}
		}
	}
	return parameters
}

func kotlinParameterOf(r *reading, parameter *gotreesitter.Node) Parameter {
	var declared Parameter
	for i := 0; i < parameter.NamedChildCount(); i++ {
		child := parameter.NamedChild(i)
		switch {
		case r.typeOf(child) == kotlinName && declared.Name == "":
			declared.Name = r.text(child)
		case declared.Type == "":
			declared.Type = simpleType(r.text(child))
		}
	}
	return declared
}

// simpleType reduces a declared type to the name of its class: package prefixes,
// generic arguments and nullability are dropped.
func simpleType(declared string) string {
	name := strings.TrimSpace(declared)
	if open := strings.Index(name, genericOpen); open >= 0 {
		name = name[:open]
	}
	name = strings.TrimSuffix(name, nullableMark)
	if dot := strings.LastIndex(name, packageSeparator); dot >= 0 {
		name = name[dot+len(packageSeparator):]
	}
	return strings.TrimSpace(name)
}

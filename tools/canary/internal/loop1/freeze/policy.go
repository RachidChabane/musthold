package freeze

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// The members of PREREG §2's table of module defaults, by module, as the Go
// source of each module's policy package declares them.
var ExpectedPolicy = map[string]map[string]string{
	"discpower": {
		"k_op": "400", "max_rounds": "3", "min_valid": "3", "min_sites": "2", "max_host_tickets": "40",
		"judge_k": "2", "decide_k": "3", "tau_judge": "0.7", "tau_screen": "0.2", "min_isolation": "restricted",
	},
	"alignment": {
		"align_k": "5", "modal_min": "0.8", "tau_route": "0.9", "theta_own": "0.5", "theta_other": "0.2",
		"theta_none": "0.2", "shadow_rate": "0.1", "max_reasoning_calls": "40", "decide_k": "3", "min_isolation": "restricted",
	},
	"completeness": {
		"tau_adv": "0.5", "decide_k": "3", "max_hunk_bytes": "6000", "max_hunks": "50",
	},
}

// Where the defaults are declared at a commit.
const (
	policyFileFormat = "%s/internal/domain/policy/policy.go"
	formatDir        = "format"
	defaultsFunc     = "Default"
	formatQualifier  = "format"
	thresholdFunc    = "UniformThreshold"
	goSuffix         = ".go"
	testSuffix       = "_test.go"
)

// ErrPolicy is returned when a module's default cannot be read from its source.
var ErrPolicy = errors.New("a policy default cannot be read from the source")

// SourceTree reads files of the repository at a commit.
type SourceTree interface {
	Show(ctx context.Context, rev, file string) ([]byte, error)
	TreeFiles(ctx context.Context, rev string, paths ...string) ([]string, error)
}

// ReadPolicy reads, at a commit, the defaults PREREG §2's table lists from the
// Default function of each module's policy package, resolving constants of the
// package and of the format package.
func ReadPolicy(ctx context.Context, source SourceTree, commit string) (map[string]map[string]string, error) {
	formatConsts, err := formatConstants(ctx, source, commit)
	if err != nil {
		return nil, err
	}
	policy := map[string]map[string]string{}
	for module, members := range ExpectedPolicy {
		file := fmt.Sprintf(policyFileFormat, module)
		data, err := source.Show(ctx, commit, file)
		if err != nil {
			return nil, err
		}
		values, err := defaultsOf(file, data, formatConsts)
		if err != nil {
			return nil, err
		}
		policy[module] = map[string]string{}
		for member := range members {
			field := fieldName(member)
			value, ok := values[field]
			if !ok {
				return nil, fmt.Errorf("%w: %s has no default for %s", ErrPolicy, module, field)
			}
			policy[module][member] = value
		}
	}
	return policy, nil
}

func fieldName(member string) string {
	var name strings.Builder
	for _, part := range strings.Split(member, "_") {
		name.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return name.String()
}

func formatConstants(ctx context.Context, source SourceTree, commit string) (map[string]ast.Expr, error) {
	files, err := source.TreeFiles(ctx, commit, formatDir)
	if err != nil {
		return nil, err
	}
	consts := map[string]ast.Expr{}
	for _, file := range files {
		if path.Dir(file) != formatDir || !strings.HasSuffix(file, goSuffix) || strings.HasSuffix(file, testSuffix) {
			continue
		}
		data, err := source.Show(ctx, commit, file)
		if err != nil {
			return nil, err
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, data, 0)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrPolicy, err)
		}
		collectConstants(parsed, consts)
	}
	return consts, nil
}

func collectConstants(file *ast.File, into map[string]ast.Expr) {
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value := spec.(*ast.ValueSpec)
			if len(value.Names) == len(value.Values) {
				for index, name := range value.Names {
					into[name.Name] = value.Values[index]
				}
			}
		}
	}
}

// defaultsOf evaluates the fields of the composite literal the Default function
// returns.
func defaultsOf(name string, data []byte, formatConsts map[string]ast.Expr) (map[string]string, error) {
	parsed, err := parser.ParseFile(token.NewFileSet(), name, data, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPolicy, err)
	}
	local := map[string]ast.Expr{}
	collectConstants(parsed, local)
	literal := defaultLiteral(parsed)
	if literal == nil {
		return nil, fmt.Errorf("%w: %s has no %s function returning a literal", ErrPolicy, name, defaultsFunc)
	}
	scope := evaluator{local: local, format: formatConsts}
	values := map[string]string{}
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := pair.Key.(*ast.Ident)
		if !ok {
			continue
		}
		if value, err := scope.eval(pair.Value); err == nil {
			values[key.Name] = value
		}
	}
	return values, nil
}

func defaultLiteral(file *ast.File) *ast.CompositeLit {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || function.Name.Name != defaultsFunc || function.Body == nil {
			continue
		}
		for _, statement := range function.Body.List {
			if returned, ok := statement.(*ast.ReturnStmt); ok && len(returned.Results) == 1 {
				if literal, ok := returned.Results[0].(*ast.CompositeLit); ok {
					return literal
				}
			}
		}
	}
	return nil
}

type evaluator struct {
	local, format map[string]ast.Expr
}

func (e evaluator) eval(expression ast.Expr) (string, error) {
	switch node := expression.(type) {
	case *ast.BasicLit:
		return literalValue(node)
	case *ast.Ident:
		value, ok := e.local[node.Name]
		if !ok {
			return "", fmt.Errorf("%w: constant %s", ErrPolicy, node.Name)
		}
		return e.eval(value)
	case *ast.SelectorExpr:
		qualifier, ok := node.X.(*ast.Ident)
		value, found := e.format[node.Sel.Name]
		if !ok || qualifier.Name != formatQualifier || !found {
			return "", fmt.Errorf("%w: reference %s", ErrPolicy, node.Sel.Name)
		}
		return e.eval(value)
	case *ast.CallExpr:
		selector, ok := node.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != thresholdFunc || len(node.Args) != 1 {
			return "", fmt.Errorf("%w: call", ErrPolicy)
		}
		return e.eval(node.Args[0])
	}
	return "", fmt.Errorf("%w: expression", ErrPolicy)
}

func literalValue(literal *ast.BasicLit) (string, error) {
	switch literal.Kind {
	case token.STRING:
		return strconv.Unquote(literal.Value)
	case token.INT, token.FLOAT:
		return NormalizeNumber(literal.Value)
	}
	return "", fmt.Errorf("%w: literal %s", ErrPolicy, literal.Value)
}

var decimal = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// NormalizeNumber writes a decimal number in its shortest form, so that 0.70 and
// 0.7 are one value.
func NormalizeNumber(text string) (string, error) {
	if !decimal.MatchString(text) {
		return "", fmt.Errorf("%w: number %q", ErrPolicy, text)
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return "", err
	}
	return strconv.FormatFloat(value, 'g', -1, 64), nil
}

package javascript

import (
	"errors"
	"fmt"
	"path"
	"sync"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// ErrUnsupportedFile is returned for a file whose extension no grammar covers.
var ErrUnsupportedFile = errors.New("unsupported javascript or typescript file")

// Extensions of the sources the engine handles, dot included.
const (
	extensionJavaScript       = ".js"
	extensionJSX              = ".jsx"
	extensionModuleJavaScript = ".mjs"
	extensionCommonJavaScript = ".cjs"
	extensionTypeScript       = ".ts"
	extensionTSX              = ".tsx"
)

// sourceExtensions lists the extensions the engine handles.
var sourceExtensions = []string{
	extensionJavaScript, extensionJSX, extensionModuleJavaScript, extensionCommonJavaScript,
	extensionTypeScript, extensionTSX,
}

var (
	javascriptGrammar = sync.OnceValue(grammars.JavascriptLanguage)
	typescriptGrammar = sync.OnceValue(grammars.TypescriptLanguage)
	tsxGrammar        = sync.OnceValue(grammars.TsxLanguage)
)

// grammarsByExtension maps an extension to the loader of its grammar.
var grammarsByExtension = map[string]func() *gotreesitter.Language{
	extensionJavaScript:       javascriptGrammar,
	extensionJSX:              javascriptGrammar,
	extensionModuleJavaScript: javascriptGrammar,
	extensionCommonJavaScript: javascriptGrammar,
	extensionTypeScript:       typescriptGrammar,
	extensionTSX:              tsxGrammar,
}

// grammarFor chooses the grammar from the extension of a file path.
func grammarFor(filePath string) (*gotreesitter.Language, error) {
	load, ok := grammarsByExtension[path.Ext(filePath)]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFile, filePath)
	}
	return load(), nil
}

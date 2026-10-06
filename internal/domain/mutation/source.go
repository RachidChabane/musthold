package mutation

// SourceFile is a source file given to a mutation engine: its path relative to
// the project root, in POSIX form, and its bytes.
type SourceFile struct {
	Path    string
	Content []byte
}

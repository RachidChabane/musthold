package detect

// Citation is one criterion a test cites with `@pytest.mark.verifies`.
type Citation struct {
	Criterion string `json:"criterion"`
	Line      int    `json:"line"`
}

// Fingerprint identifies an assertion or a `pytest.raises` block by the digest of
// its syntax tree; Text is its source, kept for evidence.
type Fingerprint struct {
	Hash string `json:"hash"`
	Text string `json:"text"`
}

// Test is a test function of a tree.
type Test struct {
	// ID is <file>::<function> or <file>::<class>::<method>.
	ID         string        `json:"id"`
	File       string        `json:"file"`
	Line       int           `json:"line"`
	Citations  []Citation    `json:"citations"`
	Assertions []Fingerprint `json:"assertions"`
	// Body is the digest of the function body, whatever the function is named.
	Body string `json:"body"`
}

// Construct is a use of a pytest mechanism that skips or deselects tests.
type Construct struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Kind  string `json:"kind"`
	Scope string `json:"scope"`
}

// Unparsed is a file the extraction could not read.
type Unparsed struct {
	File  string `json:"file"`
	Error string `json:"error"`
}

// Options are the values of the keys of a pytest configuration file that decide
// which tests are collected, by key.
type Options map[string]string

// Facts is what one tree holds.
type Facts struct {
	Tests      []Test             `json:"tests"`
	Constructs []Construct        `json:"constructs"`
	Config     map[string]Options `json:"config"`
	Unparsed   []Unparsed         `json:"unparsed"`
}

// Snapshot is the facts of the equipped commit and of the end of the session.
type Snapshot struct {
	Before Facts `json:"before"`
	After  Facts `json:"after"`
}

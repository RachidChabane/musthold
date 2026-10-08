package evidence

import "time"

// Reasons a session was ended by the runner and not by the harness.
const (
	KilledNone    = ""
	KilledTimeout = "timeout"
	KilledTurnCap = "turn-cap"
	// KilledInterrupted is the reason of a session ended by SIGINT or SIGTERM, which
	// is how the memory guard stops a batch.
	KilledInterrupted = "interrupted"
)

// Where a harness CLI keeps its state.
const (
	ConfigIsolated  = "isolated"
	ConfigDedicated = "dedicated"
	ConfigShared    = "shared"
)

// Session describes how one session was set up and how it ended. It is written
// by the runner and holds no verdict.
type Session struct {
	Harness     string `json:"harness"`
	Model       string `json:"model"`
	Ticket      string `json:"ticket"`
	Role        string `json:"role"`
	ProbeTicket string `json:"probeTicket"`
	// Versions maps each CLI the run used to the version line it printed.
	Versions map[string]string `json:"versions"`
	// Argv is the command line of the harness CLI, the prompt included.
	Argv     []string `json:"argv"`
	ExitCode int      `json:"exitCode"`
	// Killed is the reason the runner ended the session, empty when it ended itself.
	Killed         string `json:"killed"`
	EventCap       int    `json:"eventCap"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	// ConfigMode says where the harness CLI kept its credentials and state: in a
	// directory of the run (ConfigIsolated, ConfigDedicated) or in the user's own
	// (ConfigShared).
	ConfigMode string    `json:"configMode"`
	EditFile   string    `json:"editFile"`
	EditFrom   string    `json:"editFrom"`
	EditTo     string    `json:"editTo"`
	StartedAt  time.Time `json:"startedAt"`
	EndedAt    time.Time `json:"endedAt"`
}

// NoCall is the outcome of the probes that run before the session and make no
// model call.
type NoCall struct {
	// CapProbe is what the executor server answered to a fetch with no capability.
	CapProbe CapProbe `json:"capProbe"`
	// Servers is one outcome per executor server the module binaries started and
	// asked for its tools.
	Servers []ServerStart `json:"servers"`
	// PluginValidation is one outcome per plugin directory the harness CLI validated.
	PluginValidation []PluginValidation `json:"pluginValidation"`
	// CopilotListing is what Copilot CLI lists of the plugins and skills mounted
	// from the generated trees.
	CopilotListing *CopilotListing `json:"copilotListing,omitempty"`
	// PluginTrees is one inspection per generated Agent Plugins tree.
	PluginTrees []PluginTree `json:"pluginTrees,omitempty"`
}

// ServerStart records the start of one module's executor server: whether it
// answered the initialize handshake and listed the executor tools.
type ServerStart struct {
	Binary  string `json:"binary"`
	Started bool   `json:"started"`
	Detail  string `json:"detail"`
}

// CapProbe records a fetch_request sent to the executor server over standard
// input and output without the capability a hook would have added.
type CapProbe struct {
	Attempted bool `json:"attempted"`
	// Responded is false when the server never answered.
	Responded bool `json:"responded"`
	// Refused is true when the answer is an error.
	Refused bool `json:"refused"`
	// ServedTicket is true when the answer carries the ticket's nonce and schema.
	ServedTicket bool   `json:"servedTicket"`
	Detail       string `json:"detail"`
}

// PluginValidation is the result of the harness CLI's own validation of a plugin.
type PluginValidation struct {
	Dir      string `json:"dir"`
	ExitCode int    `json:"exitCode"`
	Output   string `json:"output"`
}

// SandboxProbe records whether the file a read-only executor was asked to create
// exists after the session.
type SandboxProbe struct {
	File   string `json:"file"`
	Exists bool   `json:"exists"`
}

// Touched is what changed, during the session, in the places a harness CLI keeps
// its state in the user's home: the entries whose path names this run, and the
// changes nothing ties to it, which another process of the user may have made.
type Touched struct {
	// Roots are the files and directories that were fingerprinted.
	Roots        []string `json:"roots"`
	Attributed   []string `json:"attributed"`
	Unattributed []string `json:"unattributed"`
	// ConfigWrites are the tables the session wrote into the user's own
	// configuration file, and what the runner did about each.
	ConfigWrites []ConfigWrite `json:"configWrites,omitempty"`
}

// ConfigWrite is a table that names the run's project, written by the session
// into a configuration file of the user's, for the runner to remove afterwards.
type ConfigWrite struct {
	Path string `json:"path"`
	// Table is the text of the table the session wrote, as the runner removed it.
	Table string `json:"table"`
	// Removed says the runner took the table out of the file.
	Removed bool `json:"removed"`
	// Verified says a re-read of the file showed the table gone and every other
	// byte of the file as it was before the removal.
	Verified bool `json:"verified"`
}

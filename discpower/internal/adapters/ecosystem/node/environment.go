package node

// noColorVariable keeps the tools' diagnostics free of terminal escape codes.
const noColorVariable = "NO_COLOR=1"

// quietEnvironment is the environment every tool of the project runs with.
func quietEnvironment() []string { return []string{noColorVariable} }

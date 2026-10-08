package evidence

// CopilotSubagent is one subagent of a Copilot CLI session, assembled from its
// subagent.started, subagent.selected and subagent.completed events.
//
// The events a subagent originates are tied to it in two explicit ways, never by
// the time between its start and its end: a tool or message event carries the
// `parentToolCallId` of the task call that launched the subagent, and every event
// of the subagent carries its `agentId` in the envelope. subagent.selected has no
// task call, so it is joined by the `agentId`.
type CopilotSubagent struct {
	// Name is the agent name the harness started.
	Name string
	// AgentID is the envelope agentId of the subagent's events.
	AgentID string
	// ToolCallID is the task call of the main agent that launched the subagent.
	ToolCallID string
	Started    CopilotEvent
	// Selected is the subagent.selected event, and HasSelected says it was seen.
	Selected    CopilotEvent
	HasSelected bool
	// Completed is the subagent.completed event, and HasCompleted says it was seen.
	Completed    CopilotEvent
	HasCompleted bool
}

// Subagents returns the subagents the stream started, in order.
func (s CopilotStream) Subagents() []CopilotSubagent {
	var subagents []CopilotSubagent
	for _, started := range s.Of(CopilotEventSubagentStarted) {
		subagent := CopilotSubagent{Name: started.AgentName, AgentID: started.AgentID, ToolCallID: started.ToolCallID, Started: started}
		for _, event := range s.Events {
			switch {
			case event.Type == CopilotEventSubagentSelected && subagent.AgentID != "" && event.AgentID == subagent.AgentID && !subagent.HasSelected:
				subagent.Selected, subagent.HasSelected = event, true
			case event.Type == CopilotEventSubagentDone && subagent.ToolCallID != "" && event.ToolCallID == subagent.ToolCallID && !subagent.HasCompleted:
				subagent.Completed, subagent.HasCompleted = event, true
			}
		}
		subagents = append(subagents, subagent)
	}
	return subagents
}

// Originated reports whether the subagent originated the event: the event names
// the task call that launched it as its parent, or carries its agentId.
func (a CopilotSubagent) Originated(event CopilotEvent) bool {
	byParent := a.ToolCallID != "" && event.ParentToolCallID == a.ToolCallID
	byEnvelope := a.AgentID != "" && event.AgentID == a.AgentID
	return byParent || byEnvelope
}

// Returned reports whether the event is the completion of the task call that
// launched the subagent: what the subagent handed back to the main agent.
func (a CopilotSubagent) Returned(event CopilotEvent) bool {
	return event.Type == CopilotEventToolComplete && a.ToolCallID != "" && event.ToolCallID == a.ToolCallID
}

// Launched reports whether the event is the call of the task tool that launched
// the subagent, which belongs to the main agent.
func (a CopilotSubagent) Launched(event CopilotEvent) bool {
	return event.Type == CopilotEventToolStart && a.ToolCallID != "" && event.ToolCallID == a.ToolCallID
}

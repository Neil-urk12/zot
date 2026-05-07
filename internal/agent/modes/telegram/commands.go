package telegram

import "strings"

// isStopCommand reports whether text should abort the active turn.
// Telegram users often type plain "stop" rather than bot-style
// "/stop"; keep this intentionally narrow so normal prompts like
// "stop doing X" still go to the agent.
func isStopCommand(text string) bool {
	return strings.EqualFold(strings.TrimSpace(text), "stop")
}

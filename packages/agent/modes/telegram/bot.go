package telegram

import (
	"context"
	"strings"

	"github.com/patriceckhart/zot/packages/agent/modes/bot"
	"github.com/patriceckhart/zot/packages/core"
)

// Bot is a thin wrapper around bot.Runner + Adapter.  It exists to
// keep the exported API stable for botcmd.go.
//
// Deprecated: Use telegram.NewAdapter + bot.NewRunner directly.
// Bot is kept for backward compatibility but is no longer used internally.
type Bot struct {
	Client     *Client
	Agent      *core.Agent
	Config     Config
	ZotHome    string
	Provider   string
	AuthMethod string
	CWD        string
	Save       func(Config) error
	RefreshCreds func() error

	runner  *bot.Runner
	adapter *Adapter
}

// Run starts the bot.  It constructs the adapter and runner on first
// call, then delegates to runner.Run.
func (b *Bot) Run(ctx context.Context) error {
	if b.adapter == nil {
		b.adapter = NewAdapter(b.Client, &b.Config, b.Save)
	}
	if b.runner == nil {
		b.runner = bot.NewRunner(b.adapter, b.Agent, bot.Config{
			ZotHome:      b.ZotHome,
			Provider:     b.Provider,
			AuthMethod:   b.AuthMethod,
			CWD:          b.CWD,
			RefreshCreds: b.RefreshCreds,
		})
	}
	return b.runner.Run(ctx)
}

// chunkMessage splits s into chunks no larger than limit runes, on line
// boundaries when possible.
func chunkMessage(s string, limit int) []string {
	if len(s) <= limit {
		return []string{s}
	}
	var out []string
	lines := strings.Split(s, "\n")
	var cur strings.Builder
	for _, l := range lines {
		if cur.Len()+len(l)+1 > limit && cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
		if len(l) > limit {
			// Line itself too long; hard-split.
			for len(l) > limit {
				out = append(out, l[:limit])
				l = l[limit:]
			}
		}
		if cur.Len() > 0 {
			cur.WriteString("\n")
		}
		cur.WriteString(l)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// isImageMIME returns true for MIME types the model can probably ingest
// as a vision input.
func isImageMIME(m string) bool {
	switch strings.ToLower(m) {
	case "image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp":
		return true
	}
	return false
}

// guessImageMIME infers a mime type from a filename suffix. Falls back
// to image/png because telegram photos are always re-encoded to jpeg
// but getFile's file_path may omit the extension.
func guessImageMIME(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	}
	return "image/jpeg"
}

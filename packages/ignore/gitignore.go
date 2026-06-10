// Package ignore provides a minimal .gitignore matcher shared across
// zot. It is intentionally small: enough to drop obvious non-source
// directories (build outputs, dependency and tool caches) from
// recursive walks, not a faithful git reimplementation.
package ignore

import (
	"os"
	"path/filepath"
	"strings"
)

// Gitignore is a minimal .gitignore matcher. It supports the common
// patterns used in real repos: blank lines, comments (#), negation (!),
// directory-only patterns (trailing /), anchored patterns (leading /),
// and the * / ? / [..] wildcards via filepath.Match. It intentionally
// does not implement ** globstar or nested per-directory .gitignore
// files.
type Gitignore struct {
	rules []rule
}

type rule struct {
	pattern  string
	negate   bool
	dirOnly  bool
	anchored bool
}

// Load reads the .gitignore at the root directory. A missing or
// unreadable file yields an empty matcher that ignores nothing.
func Load(root string) *Gitignore {
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return &Gitignore{}
	}
	return Parse(string(data))
}

// Parse builds a matcher from raw .gitignore file contents.
func Parse(data string) *Gitignore {
	g := &Gitignore{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		r := rule{pattern: trimmed}
		if strings.HasPrefix(r.pattern, "!") {
			r.negate = true
			r.pattern = r.pattern[1:]
		}
		if strings.HasSuffix(r.pattern, "/") {
			r.dirOnly = true
			r.pattern = strings.TrimSuffix(r.pattern, "/")
		}
		if strings.HasPrefix(r.pattern, "/") {
			r.anchored = true
			r.pattern = strings.TrimPrefix(r.pattern, "/")
		}
		if r.pattern == "" {
			continue
		}
		g.rules = append(g.rules, r)
	}
	return g
}

// Match reports whether the slash-separated relative path should be
// ignored. Later rules win, so a trailing negation can re-include a
// previously ignored path.
func (g *Gitignore) Match(rel string, isDir bool) bool {
	ignored := false
	for _, r := range g.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if r.matchPath(rel) {
			ignored = !r.negate
		}
	}
	return ignored
}

func (r rule) matchPath(rel string) bool {
	if r.anchored || strings.Contains(r.pattern, "/") {
		if ok, _ := filepath.Match(r.pattern, rel); ok {
			return true
		}
		// Anchored directory pattern also matches everything beneath it.
		return strings.HasPrefix(rel, r.pattern+"/")
	}
	// Unanchored: match the basename of any path component.
	base := rel
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		base = rel[i+1:]
	}
	if ok, _ := filepath.Match(r.pattern, base); ok {
		return true
	}
	// Match a directory component anywhere in the path.
	for _, part := range strings.Split(rel, "/") {
		if ok, _ := filepath.Match(r.pattern, part); ok {
			return true
		}
	}
	return false
}

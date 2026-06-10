package ignore

import "testing"

func TestParseAndMatch(t *testing.T) {
	g := Parse(lines("# comment", "", ".terraform/", ".terragrunt-cache/", "node_modules/", "*.log", "/build", "!keep.log"))

	cases := []struct {
		rel   string
		isDir bool
		want  bool
	}{
		{".terraform", true, true},
		// A dirOnly rule matches the directory itself; the walk prunes
		// descent on that match, so children are never tested. A file
		// path under it is therefore not matched directly by the rule.
		{".terraform/providers/x", false, false},
		{".terragrunt-cache", true, true},
		{"modules/.terragrunt-cache", true, true},
		{"node_modules", true, true},
		{"src/node_modules/pkg", true, true},
		{"debug.log", false, true},
		{"keep.log", false, false}, // re-included by negation
		{"build", true, true},      // anchored
		{"sub/build", true, false}, // anchored: only at root
		{"main.tf", false, false},
		{"src/app.go", false, false},
	}
	for _, c := range cases {
		if got := g.Match(c.rel, c.isDir); got != c.want {
			t.Errorf("Match(%q, dir=%v) = %v, want %v", c.rel, c.isDir, got, c.want)
		}
	}
}

func TestEmptyIgnoresNothing(t *testing.T) {
	g := Parse("")
	if g.Match("anything", false) || g.Match("dir", true) {
		t.Fatal("empty matcher should ignore nothing")
	}
}

// lines joins fixture lines with newlines for readable .gitignore
// fixtures.
func lines(ls ...string) string {
	out := ""
	for i, l := range ls {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}

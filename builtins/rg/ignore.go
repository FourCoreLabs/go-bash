package rg

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/mark3labs/go-bash/command"
)

// ignoreStatus follows just-bash's parser ordering: rules within one file
// are last-match-wins, but an ignored result in any ancestor file wins.
func ignoreStatus(c *command.Context, name string, dir, disabled bool) (bool, bool) {
	if disabled {
		return false, false
	}
	switch path.Base(name) {
	case "node_modules", ".git", ".svn", ".hg", "__pycache__", ".pytest_cache", ".mypy_cache", "venv", ".venv", ".next", ".nuxt", ".cargo":
		return true, false
	}
	var ancestors []string
	for base := path.Dir(name); ; base = path.Dir(base) {
		ancestors = append(ancestors, base)
		if base == "/" {
			break
		}
	}
	ignored, white := false, false
	for _, base := range slices.Backward(ancestors) {

		rel := strings.TrimPrefix(strings.TrimPrefix(name, base), "/")
		for _, file := range []string{".gitignore", ".rgignore", ".ignore"} {
			data, err := c.FS.ReadFile(path.Join(base, file))
			if err != nil {
				continue
			}
			fileIgnored := false
			for raw := range strings.SplitSeq(string(data), "\n") {
				rule := strings.TrimRight(raw, " \t\r")
				if rule == "" || strings.HasPrefix(rule, "#") {
					continue
				}
				negate := strings.HasPrefix(rule, "!")
				if negate {
					rule = rule[1:]
				}
				directoryOnly := strings.HasSuffix(rule, "/")
				rule = strings.TrimSuffix(rule, "/")
				if directoryOnly && !dir {
					continue
				}
				rooted := strings.HasPrefix(rule, "/") || (strings.Contains(rule, "/") && !strings.HasPrefix(rule, "**/"))
				rule = strings.TrimPrefix(rule, "/")
				re, err := regexp.Compile(ignoreRegex(rule, rooted))
				if err == nil && re.MatchString(rel) {
					fileIgnored = !negate
					if negate {
						white = true
					}
				}
			}
			ignored = ignored || fileIgnored
		}
	}
	return ignored, white
}

func ignoreRegex(pattern string, rooted bool) string {
	var b strings.Builder
	if rooted {
		b.WriteString("^")
	} else {
		b.WriteString("(?:^|/)")
	}
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '[':
			end := strings.IndexByte(pattern[i+1:], ']')
			if end >= 0 {
				end += i + 1
				class := pattern[i : end+1]
				if strings.HasPrefix(class, "[!") {
					class = "[^" + class[2:]
				}
				b.WriteString(class)
				i = end
			} else {
				b.WriteString(`\[`)
			}
		default:
			b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}
	b.WriteString("(?:/.*)?$")
	return b.String()
}

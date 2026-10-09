package allow

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Paths in Edit and Read rules are gitignore-style globs, which is how
// Claude Code reads them. A glob is matched one path segment at a time.

// segRegex turns one segment of a glob into a regular expression: * ? and
// [...] / [!...] classes, backslash escapes, and {a,b} read as alternatives
// to be safe (gitignore has no braces, but a matcher that has them must not
// get past a check).
func segRegex(seg string) string {
	rs := []rune(seg)
	var out strings.Builder
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case c == '\\' && i+1 < len(rs):
			out.WriteString(regexp.QuoteMeta(string(rs[i+1])))
			i += 2
		case c == '*':
			out.WriteString(".*")
			i++
		case c == '?':
			out.WriteString(".")
			i++
		case c == '[':
			j := i + 1
			if j < len(rs) && (rs[j] == '!' || rs[j] == '^') {
				j++
			}
			if j < len(rs) && rs[j] == ']' {
				j++
			}
			for j < len(rs) && rs[j] != ']' {
				if rs[j] == '\\' {
					j += 2
				} else {
					j++
				}
			}
			if j >= len(rs) {
				out.WriteString(regexp.QuoteMeta("["))
				i++
				continue
			}
			body := rs[i+1 : j]
			neg := len(body) > 0 && (body[0] == '!' || body[0] == '^')
			if neg {
				body = body[1:]
			}
			var cls strings.Builder
			for k := 0; k < len(body); {
				switch {
				case body[k] == '\\' && k+1 < len(body):
					cls.WriteString(classChar(body[k+1]))
					k += 2
				case body[k] == '-' && cls.Len() > 0 && k+1 < len(body):
					cls.WriteByte('-')
					k++
				default:
					cls.WriteString(classChar(body[k]))
					k++
				}
			}
			out.WriteByte('[')
			if neg {
				out.WriteByte('^')
			}
			out.WriteString(cls.String())
			out.WriteByte(']')
			i = j + 1
		case c == '{' && strings.ContainsRune(string(rs[i:]), '}'):
			j := i
			for rs[j] != '}' {
				j++
			}
			alts := strings.Split(string(rs[i+1:j]), ",")
			for n, a := range alts {
				alts[n] = segRegex(a)
			}
			out.WriteString("(?:" + strings.Join(alts, "|") + ")")
			i = j + 1
		default:
			out.WriteString(regexp.QuoteMeta(string(c)))
			i++
		}
	}
	return out.String()
}

// classChar writes one character for use inside a [...] class.
func classChar(r rune) string { return fmt.Sprintf(`\x{%x}`, r) }

var regexCache sync.Map // pattern -> *regexp.Regexp or error

// fullMatch reports whether name matches the segment pattern entirely,
// ignoring letter case. A pattern RE2 cannot compile (a reversed range,
// for example) never matches; checkPath refuses it before that matters.
func fullMatch(pattern, name string) bool {
	re, err := compile(pattern)
	return err == nil && re.MatchString(name)
}

func compile(pattern string) (*regexp.Regexp, error) {
	if v, ok := regexCache.Load(pattern); ok {
		if re, ok := v.(*regexp.Regexp); ok {
			return re, nil
		}
		return nil, v.(error)
	}
	re, err := regexp.Compile(`(?is)^(?:` + pattern + `)$`)
	if err != nil {
		regexCache.Store(pattern, err)
		return nil, err
	}
	regexCache.Store(pattern, re)
	return re, nil
}

// segments splits a path into its parts, without empty and "." parts.
func segments(path string) []string {
	var out []string
	for _, s := range strings.Split(path, "/") {
		if s != "" && s != "." {
			out = append(out, s)
		}
	}
	return out
}

// hasWildcard reports whether a segment has an unescaped * or ?.
func hasWildcard(seg string) bool {
	rs := []rune(seg)
	for i, c := range rs {
		if (c == '*' || c == '?') && (i == 0 || rs[i-1] != '\\') {
			return true
		}
	}
	return false
}

// spells reports whether a segment with no * or ? names exactly name,
// reading classes, escapes and braces.
func spells(seg, name string) bool {
	return !hasWildcard(seg) && fullMatch(segRegex(seg), name)
}

// anyName, as a segment of a target, stands for any name: a rule reaches
// ~/.cadre/<anyName>/members whatever it names in that place, so cadres
// made later are covered too. No file can have this name.
const anyName = "\x00"

// reaches reports whether glob matches target, a folder holding it, or,
// when target is a folder (ends in /), anything inside it.
func reaches(glob, target string) bool {
	rule, want := segments(glob), segments(target)
	folder := strings.HasSuffix(target, "/")
	type key struct{ i, j int }
	seen := map[key]bool{}
	var walk func(i, j int) bool
	walk = func(i, j int) bool {
		k := key{i, j}
		if v, ok := seen[k]; ok {
			return v
		}
		var v bool
		switch {
		case i == len(rule):
			v = true
		case j == len(want):
			v = folder
			if !v {
				v = true
				for _, x := range rule[i:] {
					if x != "**" {
						v = false
					}
				}
			}
		case rule[i] == "**":
			v = walk(i+1, j) || walk(i, j+1)
		case want[j] == anyName:
			v = walk(i+1, j+1)
		default:
			v = fullMatch(segRegex(rule[i]), want[j]) && walk(i+1, j+1)
		}
		seen[k] = v
		return v
	}
	return walk(0, 0)
}

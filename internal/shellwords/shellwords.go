// Package shellwords splits a command line into words the way a POSIX
// shell (and Python's shlex.split) does, without running anything.
package shellwords

import "errors"

// ErrUnclosed is returned for an unclosed quote or a trailing backslash.
var ErrUnclosed = errors.New("unclosed quote or trailing backslash")

// Split returns the words of s. Single quotes keep everything literally;
// double quotes keep everything except that a backslash escapes " and \;
// outside quotes a backslash escapes any character. # is an ordinary
// character: there are no comments. This matches shlex.split in posix
// mode, which the bash version of cadrei used.
func Split(s string) ([]string, error) {
	var words []string
	var cur []rune
	started := false
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if started {
				words = append(words, string(cur))
				cur, started = cur[:0:0], false
			}
		case c == '\\':
			if i+1 >= len(rs) {
				return nil, ErrUnclosed
			}
			i++
			cur, started = append(cur, rs[i]), true
		case c == '\'':
			started = true
			for i++; ; i++ {
				if i >= len(rs) {
					return nil, ErrUnclosed
				}
				if rs[i] == '\'' {
					break
				}
				cur = append(cur, rs[i])
			}
		case c == '"':
			started = true
			for i++; ; i++ {
				if i >= len(rs) {
					return nil, ErrUnclosed
				}
				if rs[i] == '"' {
					break
				}
				if rs[i] == '\\' && i+1 < len(rs) && (rs[i+1] == '"' || rs[i+1] == '\\') {
					i++
				}
				cur = append(cur, rs[i])
			}
		default:
			cur, started = append(cur, c), true
		}
	}
	if started {
		words = append(words, string(cur))
	}
	return words, nil
}

// HasOperator reports whether s has a shell control operator (;, ;;, &,
// &&, |, || or |&) outside quotes. Quoting follows Split, and # is never a
// comment. A run of punctuation is read with the shell's own operators, so
// redirections such as 2>&1, &> and >| are not operators, while the ; in
// ;> is. An unclosed quote or a trailing backslash makes any ;, & or | in
// s count.
func HasOperator(s string) bool {
	rs := []rune(s)
	any := false
	for _, c := range rs {
		if c == ';' || c == '&' || c == '|' {
			any = true
		}
	}
	for i := 0; i < len(rs); i++ {
		switch c := rs[i]; {
		case c == '\\':
			if i+1 >= len(rs) {
				return any
			}
			i++
		case c == '\'':
			for i++; ; i++ {
				if i >= len(rs) {
					return any
				}
				if rs[i] == '\'' {
					break
				}
			}
		case c == '"':
			for i++; ; i++ {
				if i >= len(rs) {
					return any
				}
				if rs[i] == '"' {
					break
				}
				if rs[i] == '\\' && i+1 < len(rs) && (rs[i+1] == '"' || rs[i+1] == '\\') {
					i++
				}
			}
		case isPunct(c):
			j := i
			for j < len(rs) && isPunct(rs[j]) {
				j++
			}
			if controlIn(string(rs[i:j])) {
				return true
			}
			i = j - 1
		}
	}
	return false
}

func isPunct(c rune) bool {
	switch c {
	case ';', '&', '|', '<', '>', '(', ')':
		return true
	}
	return false
}

// shellOps are the shell's operators made of punctuation, longest first.
var shellOps = []string{"&>>", "<<<", "&>", ">&", "<&", ">|", ">>", "<<", "<>", ";;", "&&", "||", "|&",
	";", "&", "|", "<", ">", "(", ")"}

// controlIn reads a run of punctuation as shell operators, longest first,
// and reports whether one of them separates commands.
func controlIn(run string) bool {
	for run != "" {
		for _, op := range shellOps {
			if len(run) >= len(op) && run[:len(op)] == op {
				switch op {
				case ";", ";;", "&", "&&", "|", "||", "|&":
					return true
				}
				run = run[len(op):]
				break
			}
		}
	}
	return false
}

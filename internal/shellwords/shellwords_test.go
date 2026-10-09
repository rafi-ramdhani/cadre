package shellwords

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{`a b  c`, []string{"a", "b", "c"}},
		{`bash /x/my\ cadrei/bin/orchestrator-hook.sh`, []string{"bash", "/x/my cadrei/bin/orchestrator-hook.sh"}},
		{`'a b' "c d"`, []string{"a b", "c d"}},
		{`'it''s'`, []string{"its"}},
		{`"x\"y" "a\\b" "a\b"`, []string{`x"y`, `a\b`, `a\b`}},
		{`'a\b'`, []string{`a\b`}},
		{`ba""sh`, []string{"bash"}},
		{`B\ash`, []string{"Bash"}},
		{`'' x`, []string{"", "x"}},
		{`echo x#; y`, []string{"echo", "x#;", "y"}},
		{``, nil},
	} {
		got, err := Split(tc.in)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Split(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, in := range []string{`'open`, `"open`, `trailing\`} {
		if _, err := Split(in); err == nil {
			t.Errorf("Split(%q) succeeded", in)
		}
	}
}

func TestHasOperator(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{`npm test`, false},
		{`npm test && x`, true},
		{`a;b`, true},
		{`a | b`, true},
		{`a |& b`, true},
		{`sleep 1 &`, true},
		{`grep -E 'a|b' x`, false},
		{`git commit -m "fix; typo"`, false},
		{`echo a\;b`, false},
		{`echo x#; bash`, true}, // # is never a comment
		{`echo #; bash`, true},  // stricter than bash, which reads a comment here
		{`echo 'x#'; bash`, true},
		{`echo "x\\"; bash`, true}, // \\ in double quotes is one backslash; the quote closes
		{`echo 'x\'; bash`, true},  // a backslash is literal in single quotes
		{`echo "open; x`, true},    // unclosed: any operator counts
		{`echo "open x`, false},
		{`npm test 2>&1`, false}, // a redirection, not an operator
		{`x &> f`, false},
		{`x >| f`, false},
		{`x >> f`, false},
		{`npm test ;>x bash`, true}, // the ; is real; shlex read ;> as one token
		{`a &>b; c`, true},
		{`a ;; b`, true},
		{`a $(b) c`, false}, // parentheses are not operators, as in the bash version
	} {
		if got := HasOperator(tc.in); got != tc.want {
			t.Errorf("HasOperator(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func FuzzSplit(f *testing.F) {
	for _, s := range []string{`a 'b c' "d\"e" f\ g`, `x#; y`, `'`, `"\\`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		Split(s)
		HasOperator(s)
	})
}

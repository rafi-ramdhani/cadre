package shellwords

import (
	"bufio"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The reviewer's differential test (cadrei-review-kit opfz-*): bash itself
// decides whether "a <punctuation> b" chains commands. With a and b
// defined as functions, b runs only when a control operator separated it
// from a. A miss is bash running b while HasOperator says there is no
// operator: cadrei would then accept a real chain. Over-refusals (syntax
// errors, quoted punctuation) are fine.
//
// It runs 3,000 cases by default and the reviewer's full 30,000 with
// CADREI_FULL_ORACLE=1; it is skipped without bash or with -short.
func TestHasOperatorAgainstBash(t *testing.T) {
	if testing.Short() {
		t.Skip("bash oracle is slow")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	n := 3000
	if os.Getenv("CADREI_FULL_ORACLE") == "1" {
		n = 30000
	}
	cases := generate(n)
	sandbox := t.TempDir()
	// The oracle loop from opfz-bash-oracle.sh: eval each line in a
	// subshell with a and b as functions; "true" when b ran. Words are only
	// a, x, b and 1, so nothing real runs; redirections write into the
	// sandbox.
	script := `cd "$1"
while IFS= read -r s; do
  rm -f ran; ( a() { :; }; b() { echo 1 > ran; }; eval "$s" ) </dev/null >/dev/null 2>&1
  wait 2>/dev/null
  if [ -f ran ]; then echo true; else echo false; fi
done`
	cmd := exec.Command(bash, "-c", script, "oracle", sandbox)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + sandbox}
	cmd.Stdin = strings.NewReader(strings.Join(cases, "\n") + "\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bash: %v", err)
	}
	var answers []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		answers = append(answers, sc.Text())
	}
	if len(answers) != len(cases) {
		t.Fatalf("bash answered %d of %d cases", len(answers), len(cases))
	}
	chains, misses := 0, 0
	for i, c := range cases {
		if answers[i] != "true" {
			continue
		}
		chains++
		if !HasOperator(c) {
			misses++
			if misses <= 20 {
				t.Errorf("bash runs b in %q, but HasOperator finds no operator", c)
			}
		}
	}
	t.Logf("%d cases, %d real chains, %d missed", len(cases), chains, misses)
	if chains == 0 {
		t.Error("no case chained: the oracle is not working")
	}
}

// generate makes n distinct cases "a <1 to 7 characters> b" over the
// reviewer's alphabet, from a fixed seed.
func generate(n int) []string {
	alpha := []string{"a", "x", " ", " ", "'", `"`, `\`, ";", "&", "|", "<", ">", "#", "$", "1", "-", "(", ")", "!"}
	r := rand.New(rand.NewSource(14))
	seen := map[string]bool{}
	var out []string
	for len(out) < n {
		var b strings.Builder
		for i := r.Intn(7) + 1; i > 0; i-- {
			b.WriteString(alpha[r.Intn(len(alpha))])
		}
		c := "a " + b.String() + " b"
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

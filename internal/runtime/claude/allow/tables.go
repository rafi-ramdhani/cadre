package allow

import (
	"regexp"
	"strings"
)

// The lists below are the bash 0.2.0 checker's, unchanged. Every addition
// came from a review; testdata/ holds the inputs that motivated them.

// known are Claude Code's tool names, in the letter case it uses.
var known = []string{"Bash", "PowerShell", "Edit", "Write", "Read", "WebFetch", "WebSearch",
	"NotebookEdit", "Glob", "Grep", "Agent", "Task", "Skill"}

// bare are tools that, with no specifier, allow every use.
var bare = set("bash powershell edit write webfetch read notebookedit")

// runners run code or another program given to them.
var runners = set(`bash sh zsh dash ksh mksh fish csh tcsh pwsh powershell python python3
    ruby perl php lua tclsh node deno bun bunx npx pnpx uvx pipx osascript awk gawk mawk
    eval exec sudo doas su env xargs command builtin nohup timeout nice time stdbuf script
    busybox watch iex invoke-expression start-process cmd tmux screen ssh arch setsid flock
    chroot expect caffeinate open sqlite3 java vim vi nvim emacs nix-shell pypy pypy3 ipython
    ts-node tsx zx rscript julia swift xcrun sandbox-exec gdb lldb strace ltrace dtrace parallel
    systemd-run at batch pkexec`)

// subrunners run whatever follows one of their subcommands.
var subrunners = map[string]map[string]bool{
	"docker": set("run exec"), "podman": set("run exec"), "kubectl": set("exec run"),
	"direnv": set("exec"), "devbox": set("run"), "mise": set("exec x"), "nix": set("run shell develop"),
	"uv": set("run"), "poetry": set("run"), "pdm": set("run"), "pipenv": set("run"), "hatch": set("run"),
	"conda": set("run"), "npm": set("exec x"), "pnpm": set("dlx exec"), "yarn": set("dlx exec"),
	"bun": set("x"), "cargo": set("run"), "go": set("run"), "bundle": set("exec"), "rbenv": set("exec"),
	"pyenv": set("exec"), "dotnet": set("run exec"),
}

// fromFiles run code from project files a member can change; a nil set
// means any subcommand.
var fromFiles = map[string]map[string]bool{
	"make": nil, "gmake": nil, "just": nil, "rake": nil, "gradle": nil, "mvn": nil,
	"npm": set("run test start"), "pnpm": set("run test start"), "yarn": set("run test start"),
	"pip": set("install"), "pip3": set("install"),
}

// protected are files Claude Code never pre-approves writes to.
var protected = []string{".bashrc", ".bash_profile", ".bash_login", ".bash_aliases", ".zshrc", ".zprofile",
	".zshenv", ".zlogin", ".profile", ".envrc", ".gitconfig", ".gitmodules", ".npmrc",
	".yarnrc", ".yarnrc.yml", ".mcp.json", ".claude.json"}

// outsideSuffixes are folders, wherever they are, whose files run code
// outside a member's session or hold cadrei's own state.
var outsideSuffixes = []string{".local/bin", ".config/cadre", ".cache/cadrei", ".config/fish", "library/launchagents",
	".config/autostart", ".config/systemd/user"}

// outsideNames are names that, as any part of a path, mean the same.
var outsideNames = []string{".ssh", ".tmux.conf", ".vimrc", "launchagents", "crontab"}

var versioned = regexp.MustCompile(`^(?:python|ruby|perl|php|lua|node|pip)\d+(?:\.\d+)*$`)

var blanket = []string{"anything", "any command", "all files", "everything", "any file", "all commands"}

// autoRefused: --auto text may not argue about permissions or claim the
// user's approval.
var autoRefused = regexp.MustCompile(
	`\bpermissions?\b|\bsettings\b|settings\.json|soft_deny|\bden(y|ies|ied)\b|\.claude|\bgrant(s|ed)?\b` +
		`|cadrei\W*allow|\ballow subcommand|pre-?approved|approved (by|in advance)|\brules?\b|\bmay do\b|dot-?claude` +
		`|zshrc|bashrc|bash_profile|zprofile|zshenv|\.profile|gitconfig|gitmodules|npmrc|envrc|\.ssh|authorized_keys` +
		`|launchagents|crontab|tmux\.conf|vimrc|\.config/fish|autostart|systemd|\.local/bin|\.mcp\.json|\.claude\.json` +
		`|\b(user|owner|human)'?s?\b.{0,40}\b(wish|wants?|ok with|fine with)\b` +
		`|\baccess list\b|\b(their|its) own access\b|\bdot\W*claude\b|cadrei[\W_]*conf` +
		`|\b(user|owner|human|you)\b.{0,40}\b(approv|consent|authori[sz]|agree)`)

var cadreiAllow = regexp.MustCompile(`cadrei\W*allow`)

func set(words string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}

# Homebrew formula

`cadre.rb.in` is the formula for the tap `rafi-ramdhani/homebrew-cadre`. It installs the release binary for the machine (no build, no bottles), depends on tmux (and git on Linux), and points to Claude Code's cask in its caveats.

`formula.sh <version> <checksums.txt>` fills it in from a release. The release workflow (`.github/workflows/release.yml`) runs it on every `v*` tag and attaches the result, `cadre.rb`, to the GitHub release. CI checks that it builds from the snapshot's checksums.

Nothing here writes to the tap. At release:

1. Create the tap repository `rafi-ramdhani/homebrew-cadre`, once, after the user's confirmation.
2. Copy the release's `cadre.rb` to `Formula/cadre.rb` in the tap, and open a pull request there.
3. In the tap, `brew audit --strict --online cadre`, `brew install cadre` and `brew test cadre` on macOS and Linux.

Users then run `brew install rafi-ramdhani/cadre/cadre`, and upgrade with `brew upgrade cadre`.

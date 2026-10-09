// Package cadrei holds the files the cadrei binary carries with it: the
// orchestrator skill, the member protocol and the template a new cadrei
// starts from. They are written out where they are needed (section O.2).
package cadrei

import "embed"

// Assets is the embedded skill, protocol, orchestrator text and template.
// The template is embedded with all: so its dot files (.gitignore,
// teams/.gitkeep) come along. orchestrator.md is the one source of the
// orchestrator's instructions, for its prompt and for the hook (K.3).
//
//go:embed skills/cadrei/SKILL.md protocol.md orchestrator.md all:template
var Assets embed.FS

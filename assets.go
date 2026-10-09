// Package cadre holds the files the cadre binary carries with it: the
// orchestrator skill, the persona protocol and the template a new cadre
// starts from. They are written out where they are needed (section O.2).
package cadre

import "embed"

// Assets is the embedded skill, protocol, orchestrator text and template.
// The template is embedded with all: so its dot files (.gitignore,
// teams/.gitkeep) come along. orchestrator.md is the one source of the
// orchestrator's instructions, for its prompt and for the hook (K.3).
//
//go:embed skills/cadre/SKILL.md protocol.md orchestrator.md all:template
var Assets embed.FS

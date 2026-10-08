// Package cadre holds the files the cadre binary carries with it: the
// orchestrator skill, the persona protocol and the template a new cadre
// starts from. They are written out where they are needed (section O.2).
package cadre

import "embed"

// Assets is the embedded skill, protocol and template. The template is
// embedded with all: so its dot files (.gitignore, teams/.gitkeep) come along.
//
//go:embed skills/cadre/SKILL.md protocol.md all:template
var Assets embed.FS

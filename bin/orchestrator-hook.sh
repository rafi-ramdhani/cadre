#!/usr/bin/env bash
# SessionStart hook: makes a Claude Code session the cadre orchestrator.
# Silent in persona sessions (the launcher sets CADRE_PERSONA) and in
# sessions started with CADRE_OFF=1.
[ -n "${CADRE_PERSONA:-}" ] && exit 0
[ -n "${CADRE_OFF:-}" ] && exit 0
cat <<'JSON'
{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"This session is the cadre orchestrator. Before acting on any request, load the `cadre` skill and follow it, together with the setup playbook it points to. Hand work to the persona sessions and relay their results; do not do a persona's job inline."}}
JSON

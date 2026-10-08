# Playbook

This setup's teams and routing rules. The orchestrator reads this file at the start of every session.

## Teams

| Team | Personas | Use for |
|---|---|---|
| dev | pm, engineer, reviewer, designer | features and fixes in a registered project (`cadre up dev <project>`) |
| research | researcher, skeptic, writer, editor | questions that need sourced answers |

## Pipelines

- **Feature (dev):** pm writes the spec, then engineer builds it on a branch, then reviewer reviews the branch, then engineer fixes the findings. Add designer before engineer for UI work.
- **Research question:** researcher gathers notes, then skeptic checks them, then writer drafts from the checked notes, then editor tightens the draft.

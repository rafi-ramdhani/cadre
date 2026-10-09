# Playbook

This cadre's teams and routing rules. The orchestrator reads this file at the start of every session.

## Teams

| Team | Personas | Use for |
|---|---|---|
| dev | engineer, reviewer | features and fixes in a project (`cadre up dev <project>`) |

## Pipelines

- **Feature (dev):** engineer builds it on a branch, then reviewer reviews the branch, then engineer fixes the findings.

The user adds teams and personas by asking the orchestrator, which writes `personas/<team>/<role>.md` and updates this playbook.

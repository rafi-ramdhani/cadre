# {{name}}

A cadre: Claude Code persona sessions, run by one orchestrator, working on the projects in `projects/`.

- `playbook.md`: teams, pipelines and routing rules (the orchestrator reads it every session)
- `projects.yaml`: every project, linked by repo; each is cloned into `projects/<name>`
- `personas/<team>/<role>.md`: one persona per file
- `teams/<team>/`: notes and outputs of teams without their own folder
- `cadre.conf`: settings such as the persona permission mode

Grow it as you need: `cadre add project`, `cadre add team`, `cadre add persona`, or edit the files directly.

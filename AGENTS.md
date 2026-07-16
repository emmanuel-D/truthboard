<!-- truthboard:begin -->
## Truthboard working agreement

This repo tracks work with Truthboard. One rule: **statuses are derived
from git, never typed** — there is no way to set one, so don't look for it.

Before any coding task:

1. Check the board first: `get_board` (MCP) or `truthboard audit` —
   don't duplicate work that is already in flight.
2. Get your task: `get_brief <id>` returns the story's goal, acceptance
   criteria, scope, and linking instructions. For new work, `create_spec`
   with a full goal and acceptance body — never leave the placeholder.
3. Work on a branch containing the spec id (e.g. `feature/tb-1234-slug`).
4. End **every** commit message with the trailer line: `Spec: tb-1234`.
   That trailer is how your work appears on the board with zero extra effort.
5. When acceptance criteria are met, merge — the board flips to done by
   itself. Reverts and red CI flip it to regressed by themselves too.

Story *intent* (title, goal, acceptance, epic, priority, scope) is always
editable — `update_spec` over MCP, the CLI, or editing
`.truthboard/specs/*.md` directly. Commit intent changes like code.
<!-- truthboard:end -->

---
name: propose-memory
description: Create one explicit memory replacement proposal for human review.
disable-model-invocation: true
allowed-tools: mcp__git-push-memories__memory_propose_replace
argument-hint: "[memory fact to propose]"
---

Create a proposal only because the user explicitly invoked this skill.

Before calling a tool, obtain exactly these structured values from the user or current task evidence:

- `target_path`: canonical relative Markdown path in the configured memory repository;
- `section_id`: existing stable section anchor ID;
- `replacement`: complete replacement body for that section;
- `source_paths`: canonical relative code paths that support the proposal, or an empty array.

If `target_path` or `section_id` is unknown, ask the user. Never infer either value from free-form text alone.

Call `mcp__git-push-memories__memory_propose_replace` exactly once with those four fields. Do not retry automatically. Do not call apply or publish.

Return the proposal ID, revision, target, and diff summary. Tell the user that Markdown and Git remain unchanged until explicit CLI review and apply.

If the MCP tool is absent or returns an error, report that error visibly and stop. Never persist the candidate in a file, log, spool, issue, session note, or alternate tool.

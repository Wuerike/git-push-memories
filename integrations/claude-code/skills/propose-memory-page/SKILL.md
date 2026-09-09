---
name: propose-memory-page
description: Create one explicit new-page memory proposal for human review.
disable-model-invocation: true
allowed-tools: mcp__git-push-memories__memory_propose_create_page
argument-hint: "[memory fact for a new page]"
---

Create a proposal only because the user explicitly invoked this skill.

Obtain a `memories/<lowercase-kebab-case>.md` `target_path`, a human-readable `title`, a plain-text or inline-Markdown `body`, and supporting canonical `source_paths`. The body contains no heading or anchor; the product generates document structure and IDs.

Show those values and receive confirmation. Then call `mcp__git-push-memories__memory_propose_create_page` exactly once. Return its proposal ID, proposal branch, revision, operation, target, diff summary, and the commands for optional branch push, human review, and apply. On tool failure, report the error and stop without a fallback write.

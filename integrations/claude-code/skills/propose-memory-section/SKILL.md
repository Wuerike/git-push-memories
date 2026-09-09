---
name: propose-memory-section
description: Create one explicit new-section memory proposal for human review.
disable-model-invocation: true
allowed-tools: mcp__git-push-memories__memory_propose_create_section
argument-hint: "[memory fact for an existing page]"
---

Create a proposal only because the user explicitly invoked this skill.

Obtain the existing canonical `memories/*.md` `target_path`, a human-readable H2 `heading`, a plain-text or inline-Markdown `body`, and supporting canonical `source_paths`. The body contains no heading or anchor; the product generates the section ID and appends the section.

Show those values and receive confirmation. Then call `mcp__git-push-memories__memory_propose_create_section` exactly once. Return its proposal ID, proposal branch, revision, operation, target, diff summary, and the commands for optional branch push, human review, and apply. On tool failure, report the error and stop without a fallback write.

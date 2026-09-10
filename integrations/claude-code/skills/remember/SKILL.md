---
name: remember
description: Search memory and create one explicit proposal for a fact after human confirmation.
disable-model-invocation: true
allowed-tools: mcp__git-push-memories__memory_search, mcp__git-push-memories__memory_expand, mcp__git-push-memories__memory_propose
argument-hint: "[fact to remember]"
---
<!-- managed-by: git-push-memories -->

Handle one fact only because the user explicitly invoked `/remember`.

1. Extract a short retrieval query from the fact and call `memory_search` once with `budget_bytes: 2048`. Keep the fact, query, results, and reasoning in memory only.
2. Expand every handle returned by the bounded search response, in rank order, up to a maximum of three, with `budget_bytes: 4096`. Do not stop before those returned candidates are classified.
3. Use the search only to suggest how a future curator should treat the fact:

| Search result | Suggested action and target |
|---|---|
| no related memory | `create`, with no target unless the user supplied one |
| related memory should change | `update`, targeting its canonical Markdown path |
| related memory is obsolete | `remove`, targeting its canonical Markdown path |
| fact already represented by current content | cancel |

Search is guidance, not authoritative duplicate or contradiction detection. When results are ambiguous, explain the ambiguity and leave the target empty rather than inventing one.

4. Prepare a concise `title`, one `suggested_action` (`create`, `update`, or `remove`), an optional canonical `target`, the self-contained `knowledge`, and only canonical code `source_paths` that support it. Do not include prompts, responses, transcript references, session IDs, or private reasoning.
5. Show those exact fields and require explicit confirmation to create the proposal.
6. After confirmation, call `memory_propose` exactly once. Cancellation, insufficient budget, missing MCP server, or any tool error ends the flow without another proposal call or fallback write. If the tool returns `not_on_main`, explain that the memory repository must be checked out on exactly `main` before a new explicit attempt.
7. Return the proposal ID, Markdown path, and commit. If `duplicate` is true, explain that the exact proposal already existed and no new file or commit was created. Otherwise explain that the proposal is now committed locally on `main`. Pushing remains a human action:

```bash
git -C <memory-repository> push
```

The proposal call receives only its closed fields; all session-private material remains outside tool arguments and durable evidence. Curating proposals into canonical memory is a separate flow.

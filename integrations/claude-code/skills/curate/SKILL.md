---
name: curate
description: Review one committed create proposal and curate it into a canonical page or section after confirmation.
disable-model-invocation: true
allowed-tools: mcp__git-push-memories__memory_proposal_list, mcp__git-push-memories__memory_search, mcp__git-push-memories__memory_expand, mcp__git-push-memories__memory_curate_preview, mcp__git-push-memories__memory_curate_apply
argument-hint: "[proposal ID]"
---
<!-- managed-by: git-push-memories -->

Curate one proposal only because the user explicitly invoked `/curate`. This curation phase supports only proposals whose suggested action is `create`.

1. Call `memory_proposal_list` exactly once. Do not treat a proposal as canonical memory.
2. If the user supplied a proposal ID, select only its exact match. Otherwise show the available proposal IDs, titles, suggested actions, and targets, then ask the user to select one. Cancellation, no proposals, an unknown ID, or selection of an unsupported action ends the flow without preview or apply.
3. Show the selected proposal's exact knowledge and source paths. Derive a short query from that knowledge and call `memory_search` exactly once with `budget_bytes: 4096`. Expand every returned handle, in rank order, up to three, with `budget_bytes: 4096`.
4. Classify the candidates. Choose `create_section` with an existing canonical page and a concise new heading when that page should own the knowledge. Choose `create_page` with a new `memories/<category>/<lowercase-kebab-case>.md` path when no existing page should own it. A proposal target is only a suggestion. If the fact is already represented or should update existing knowledge, explain that this phase cannot apply it and stop.
5. Then call `memory_curate_preview` exactly once with `operation`, the selected proposal ID, target path, and `heading` only for `create_section`. Any tool error ends the flow without apply or fallback writes.
6. Show the exact operation, proposal ID, title, suggested action, knowledge, source paths, target path, heading when present, section ID, related canonical memory candidates, and diff returned by the preview. Treat related memory as evidence to consider, not an authoritative duplicate or contradiction decision. If it changes the classification, stop and require a new explicit attempt. Otherwise require explicit confirmation that this exact diff may be applied and committed.
7. After confirmation, call `memory_curate_apply` exactly once with only the returned preview token. Cancellation or any adjustment ends the flow; start a new explicit `/curate` attempt to produce another preview.
8. Return the proposal ID, canonical target, section ID, and commit. Explain that the commit contains the canonical change and removal of the consumed proposal. Pushing remains a human action:

```bash
git -C <memory-repository> push
```

Never call apply without a preview from this invocation or before the user confirms its exact diff. Never edit the memory repository through another tool.

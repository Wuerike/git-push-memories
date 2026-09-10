---
name: curate
description: Review one committed create proposal and curate it into canonical memory after confirmation.
disable-model-invocation: true
allowed-tools: mcp__git-push-memories__memory_proposal_list, mcp__git-push-memories__memory_curate_preview, mcp__git-push-memories__memory_curate_apply
argument-hint: "[proposal ID]"
---
<!-- managed-by: git-push-memories -->

Curate one proposal only because the user explicitly invoked `/curate`. This first curation phase supports only proposals whose suggested action is `create`.

1. Call `memory_proposal_list` exactly once. Do not treat a proposal as canonical memory.
2. If the user supplied a proposal ID, select only its exact match. Otherwise show the available proposal IDs, titles, suggested actions, and targets, then ask the user to select one. Cancellation, no proposals, an unknown ID, or selection of an unsupported action ends the flow without preview or apply.
3. Show the selected proposal's exact knowledge and source paths. Then choose a new Markdown page path under one of the configured categories returned by the list. Use `memories/<category>/<lowercase-kebab-case>.md`. A proposal target is only a suggestion; do not use a target outside the returned categories or invent a different fact.
4. Then call `memory_curate_preview` exactly once with the selected proposal ID and target path. Any tool error ends the flow without apply or fallback writes.
5. Show the exact proposal ID, title, suggested action, knowledge, source paths, target path, section ID, related canonical memory candidates, and diff returned by the preview. Treat related memory as evidence to consider, not an authoritative duplicate or contradiction decision. If the proposal is already represented or should modify existing memory, explain that this phase cannot apply it and stop. Otherwise require explicit confirmation that this exact diff may be applied and committed.
6. After confirmation, call `memory_curate_apply` exactly once with only the returned preview token. Cancellation or any adjustment ends the flow; start a new explicit `/curate` attempt to produce another preview.
7. Return the proposal ID, canonical target, section ID, and commit. Explain that the commit contains the canonical page and removal of the consumed proposal. Pushing remains a human action:

```bash
git -C <memory-repository> push
```

Never call apply without a preview from this invocation or before the user confirms its exact diff. Never edit the memory repository through another tool.

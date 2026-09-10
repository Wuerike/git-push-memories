---
name: curate
description: Review one committed proposal and consume or discard it after confirmation.
disable-model-invocation: true
allowed-tools: mcp__git-push-memories__memory_proposal_list, mcp__git-push-memories__memory_search, mcp__git-push-memories__memory_expand, mcp__git-push-memories__memory_curate_preview, mcp__git-push-memories__memory_curate_apply
argument-hint: "[proposal ID]"
---
<!-- managed-by: git-push-memories -->

Curate one proposal only because the user explicitly invoked `/curate`. A proposal may be consumed into canonical memory or discarded with a closed reason code.

1. Call `memory_proposal_list` exactly once. Do not treat a proposal as canonical memory.
2. If the user supplied a proposal ID, select only its exact match. Otherwise show the available proposal IDs, titles, suggested actions, and targets, then ask the user to select one. Cancellation, no proposals, or an unknown ID ends the flow without preview or apply.
3. Show the selected proposal's exact knowledge and source paths. Derive a short query from that knowledge and call `memory_search` exactly once with `budget_bytes: 4096`. Expand every returned handle, in rank order, up to three, with `budget_bytes: 4096`.
4. Classify the candidates. For `create`, choose `create_section` with an existing canonical page and a concise new heading when that page should own the knowledge; otherwise choose `create_page` with a new `memories/<category>/<lowercase-kebab-case>.md` path. For `update`, choose `replace_section` only when one expanded leaf section is the unambiguous target. Build a complete replacement body that incorporates the proposal while preserving every still-valid fact. For `remove`, choose `remove_section` for one unambiguous expanded leaf; choose `remove_page` only when that leaf is the sole canonical `Overview` of a generated page and the whole page is obsolete. Use the handle's exact path, section ID, and content hash. A proposal target is only a suggestion. Do not cascade removals or repair links automatically.
5. Classify each expanded canonical candidate and related proposal as `equivalent`, `contradictory`, `related`, or `unrelated`; present this as an assisted assessment, never as a completeness guarantee. Choose `discard_proposal` only when the proposal itself should not become memory, using exactly one reason: `not_relevant`, `incorrect`, `sensitive`, `wrong_target`, `superseded`, or `duplicate`. Use `duplicate` only when identified evidence already represents the same knowledge. Do not discard valid knowledge merely because its suggested target is wrong; select the correct canonical target instead.
6. If any candidate is classified as `contradictory`, show the conflicting candidate and ask the user to choose the resolution from only options supported by the selected proposal and identified evidence before requesting a preview. Any proposal may preserve canonical memory and be discarded. An `update` may replace an identified canonical leaf with a complete current state. A `create` may keep both facts only when they have distinct explicit scopes and can use `create_page` or `create_section`. Otherwise cancel; ambiguous or unsupported resolution ends the flow without preview.
7. Then call `memory_curate_preview` exactly once with `operation` and the selected proposal ID. Include target path for canonical operations; `heading` only for `create_section`; `section_id`, `content_hash`, and complete `replacement` only for `replace_section`; `section_id` and `content_hash` for `remove_section` or `remove_page`; and `reason` only for `discard_proposal`. Any tool error ends the flow without apply or fallback writes.
8. Classify and show every canonical candidate and related proposal in the preview evidence, followed by the exact operation, proposal fields, target fields, replacement or discard reason, and diff. Lexical candidates are limited evidence, not authoritative duplicate or contradiction detection. If preview evidence changes the classification or chosen resolution, stop and require a new explicit attempt. Otherwise require explicit confirmation that this exact decision and diff may be applied and committed.
9. After confirmation, call `memory_curate_apply` exactly once with only the returned preview token. Cancellation or any adjustment leaves the proposal unchanged and ends the flow; start a new explicit `/curate` attempt to produce another preview.
10. Return the proposal ID, canonical target and section ID when present, decision, reason when discarded, and commit. Explain that a consumed commit contains the exact canonical change plus proposal removal, while a discarded commit removes only the proposal and records its reason. Pushing remains a human action:

```bash
git -C <memory-repository> push
```

Never call apply without a preview from this invocation or before the user confirms its exact diff. Never edit the memory repository through another tool.

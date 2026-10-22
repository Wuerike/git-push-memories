---
name: remember
description: Interpret input, check memory and pending proposals, and create one explicit proposal after human confirmation.
disable-model-invocation: true
allowed-tools: mcp__git-push-memories__memory_search, mcp__git-push-memories__memory_expand, mcp__git-push-memories__memory_proposal_list, mcp__git-push-memories__memory_reference_snapshot, mcp__git-push-memories__memory_reference_expand, mcp__git-push-memories__memory_propose
argument-hint: "[fact to remember]"
---
<!-- managed-by: git-push-memories -->

Handle one subject only because the user explicitly invoked `/remember`. This skill always creates a new proposal; it never changes an existing one. Consolidating related proposals is the job of `/curate`.

1. **Interpret the input.** Extract the facts and identify the **subject** (the team, service, or domain, not the sentence), the **scope**, and the natural **shape** of the knowledge (member list, definition, rule, decision). If the input has facts about different subjects, ask the user to run `/remember` again for each one and continue with at most one.
2. **Check the scope.** The memory repository is shared. A fact about someone's identity or role inside a team or domain becomes knowledge about that team ("Reading layer team. Members: Wuerike"), not about the person. A personal preference or a fact that only concerns the user does not belong in shared memory: say so, ask for a decision, and never decide silently. A request to restructure an existing page is not a personal fact; it becomes an `update` proposal.
3. **Look at what exists, in both sources.** Extract a short retrieval query from the subject and call `memory_search` once with `budget_bytes: 2048`. Expand every handle returned, in rank order, up to a maximum of three, with `budget_bytes: 4096`. Call `memory_proposal_list` once for the pending proposals. Classify each candidate as `same subject` or `unrelated`. Keep the fact, query, results, and reasoning in memory only.
4. **Choose the path** and show the options with a recommendation:

| Situation | Suggested action and options |
|---|---|
| pending proposal about the same subject | warn with its title and ID; offer to create a new proposal anyway (`/curate` will group them; recommended) or cancel |
| canonical page about the subject, no pending proposal | `update`, targeting its canonical Markdown path |
| related memory is obsolete | `remove`, targeting its canonical Markdown path |
| no related memory | `create`, with no target unless the user supplied one |
| fact already represented by current content | cancel |

Search is guidance, not authoritative duplicate or contradiction detection. When results are ambiguous, explain the ambiguity and leave the target empty rather than inventing one.

5. For committed code files that support the fact, call `memory_reference_snapshot` with the explicit local Git worktree root, optional full revision, and canonical relative paths. Use only the exact `source_references` it returns; never invent or edit repository IDs, revisions, paths, or hashes. The local root is transient and must not be copied into proposal fields. If snapshotting fails, show the failure and either omit that source with the user's agreement or cancel.
6. For each returned reference, call `memory_reference_expand` from line 1 with `budget_bytes: 4096`. If truncated, request the next line range only when needed, up to three chunks per reference. Assess the exact committed content as `supports`, `contradicts`, or `unclear`. This assisted assessment is separate from deterministic integrity and is not a claim made by either reference tool. Never copy source excerpts into proposal fields.
7. **Draft the proposal.** The `title` is the subject as a noun ("Reading layer team"), never the event. The `knowledge` is self-contained, in the third person, never says "the user", and uses the natural shape of the data (for example `Members:\n- Wuerike`), without headings. Add one `suggested_action` (`create`, `update`, or `remove`), an optional canonical `target`, `source_paths: []`, and only exact structured `source_references` from the snapshot. Do not include prompts, responses, transcript references, session IDs, local repository roots, or private reasoning.
8. **Show the interpretation before the fields**: "I read this as: subject = ..., scope = ..., facts = ..., suggested structure = ...", marking what was inferred and what was literal. Then show the exact fields and the semantic assessment, and require explicit confirmation to create the proposal.
9. After confirmation, call `memory_propose` exactly once. Cancellation, insufficient budget, missing MCP server, or any tool error ends the flow without another proposal call or fallback write. If the tool returns `not_on_main`, explain that the memory repository must be checked out on exactly `main` before a new explicit attempt.
10. Return the proposal ID, Markdown path, and commit. If `duplicate` is true, explain that the exact proposal already existed and no new file or commit was created. Otherwise explain that the proposal is now committed locally on `main`. Pushing remains a human action:

```bash
git -C <memory-repository> push
```

The proposal call receives only its closed fields; all session-private material remains outside tool arguments and durable evidence. Curating proposals into canonical memory is a separate flow.

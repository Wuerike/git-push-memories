---
name: promote
description: Consolidate all pending proposals through a global plan, direct memory file edits, human review and a pull request.
disable-model-invocation: true
allowed-tools: Read, Write, Edit, Glob, Grep, Agent, mcp__git-push-memories__memory_promote_prepare, mcp__git-push-memories__memory_promote_check, mcp__git-push-memories__memory_promote_submit, mcp__git-push-memories__memory_promote_finish
---
<!-- managed-by: git-push-memories -->

Run only because the person explicitly invoked `/promote`. The principal and the person decide how knowledge becomes memory. The tools provide the workspace, canonical formatting and publication.

## 1. Prepare

Call `memory_promote_prepare`. Use the returned worktree, proposal input paths, memory inventory and selected revisions. Proposal input paths are absolute; memory paths are relative to the returned worktree. Treat proposal files as inputs for discussion.

The initial proposal set is fixed. Inputs received afterwards belong to a later consolidation. Start a fresh workspace; this version does not restore an interrupted editing session.

If a tool, read or write fails, report the error and stop. Keep prepared files in place and never claim completion after a failure. Use the promotion tools for Git and publication.

## 2. Read the entire set

Read every proposal at its `input_path` and every memory in `memory_paths` in full before preparing the global plan. Read large files in successive calls until their end and account for every inventory path. Search is only a navigation aid; it never replaces reading the complete inventory. There is no proposal batch size.

If the entire set cannot be read within the available context, say so and stop. Never present a partial analysis as complete.

## 3. Discuss the global plan

Show topic groups, proposal IDs and titles, destination pages (existing or new), and suggested changes. Several proposals can contribute to one memory; one proposal can contribute to several memories. Suggested actions and destinations are hints: select the canonical destination that best fits the knowledge.

Account for every initial proposal with one or more destinations or a free-form justification for excluding it from memory. Explain duplicates, incorrect interpretations and unsuitable knowledge openly. Let the person merge or split groups, change destinations and resolve contradictions. Update the global plan in conversation; do not create a separate plan file.

Reorganize only memories related to this proposal set, as approved in the global plan; preserve knowledge that remains valid. Leave unrelated improvements outside this consolidation.

Wait for explicit approval of the global plan before editing any canonical file.

## 4. Write final memories

Write canonical knowledge in complete final pages in the returned worktree. Omit proposal instructions about action and destination from the memory prose. Name pages after their subject, using `memories/<configured-category>/<lowercase-kebab-case>.md`. Preserve frontmatter. Preserve existing @id markers on retained sections. Leave new headings without markers; checking generates their IDs before review. Remove obsolete pages only when their removal is part of the approved plan.

After plan approval, the principal decides whether to delegate independent memory files. Assign one owner per file in each round. Each owner receives the exact assigned paths, current page, all relevant proposals, approved decisions and cross-page context. Each delegate writes only its assigned files in the same worktree. Only the principal performs Git, proposal cleanup and publication. Wait for every delegate to finish before reviewing the result.

## 5. Review the whole result

The principal rereads every prepared page and checks it against the approved global plan. Review duplicates, contradictions, references and omissions across the resulting pages. Resolve inconsistencies between delegates before presenting the files.

If content materially changes the approved plan, return to discussion and obtain the person's decision before writing that change.

Call `memory_promote_check` to complete IDs, validate canonical files and obtain the current diff. An empty canonical diff has no publication path in this version: report it and leave proposals untouched.

## 6. Review with the person

Show a global summary and the complete diff for each changed memory in your message. Give the worktree path for optional VS Code editing; do not open the editor automatically. Terminal and editor review the same final files.

Finish all agent writing before human review. Stop writing while the person is editing. Do not run check during human editing. Wait until the person asks for adjustments or says editing is finished in conversation.

For requested adjustments, reread the current files and preserve human edits outside the requested changes. New delegate rounds start from those files with explicit assignments. After edits, run check again and show the updated diff before seeking publication approval. Keep the conversation's global plan current.

## 7. Publish

Ask for explicit approval to publish the current files and diff. The approved global summary accounts for every initial proposal with its destinations or free-form justification.

Before submitting, reread the current changed files to confirm the result still matches the reviewed diff. If anything changed, return to human review and obtain a new approval. Do not edit files after publication approval.

After explicit approval, call `memory_promote_submit` with the approved global summary as `summary`. This creates one canonical commit and opens the PR. Return the commit and PR link. The person reviews and merges the PR.

## 8. Complete after merge

The next `/promote` confirms and cleans a previously merged publication before preparing a new set. The person can also run `gpm promote --finish` or ask you to call `memory_promote_finish` after merging.

Cleanup requires confirmed merge and removes only concluded inputs whose path and contents still match. New or changed proposals remain pending. Published local reads then follow main. The publication record stores only the PR/input linkage for this cleanup.

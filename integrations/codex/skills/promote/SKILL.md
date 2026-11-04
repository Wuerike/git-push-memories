---
name: promote
description: Investigate selected proposals and related memory, agree a plan, edit ordinary Markdown and publish a reviewed pull request.
---
<!-- managed-by: git-push-memories -->

Use the tools exposed by the `git-push-memories` MCP server. Tool names may
include a client prefix; select them by their `memory_*` name and server.
If the server is unavailable, keep the requested work in the conversation.
Explain that `gpm setup <repository-url> --agent codex` registers it.

Use Codex's available file and shell tools to read and edit the absolute paths
returned by preparation. The worktree and fixed inputs live outside the code
repository. If directory access is denied, stop the affected work and explain
that the person can start Codex with `--add-dir <GPM-state-directory>` using the
directory printed by setup. Keep the prepared files; never substitute a worktree
inside the code repository or change sandbox settings automatically.

Run because the person explicitly invoked `$promote`. You and the person decide
how knowledge becomes memory. The tools provide Git operations, fixed inputs,
a read-only diff and publication of ordinary Markdown files.

## 1. Select and prepare

Use `memory_proposal_list` to inspect pending proposals by `path`, `blob` and
content. Interpret their titles and subjects yourself. Follow the requested
scope; select related proposal paths for a focused consolidation. If no scope
was requested, consider all pending proposals. Explain the selected scope and
leave other proposals pending.

Call `memory_promote_prepare` with `proposal_paths` for a subset, or omit that
argument to select all pending proposals. Use its `worktree`, proposal
`input_path` values, `memory_paths`, categories and Git revisions. Input paths
are absolute; memory paths are relative to the worktree. The selected proposal
set is fixed at preparation. Later inputs stay pending for another round.

Use the promotion tools for Git and publication. Report failures and preserve
prepared files. Distinguish successful steps from an uncertain remote outcome;
never report completion after an error. Prepared editing state belongs to the
current MCP process; keep the worktree path available to the person.

## 2. Investigate relevant knowledge

Read every selected proposal at its `input_path` in full. Use the memory
inventory, directory structure, search and references to find related knowledge.
Read every page you will edit in full before proposing its changes. Read large
files in successive calls until their end.

Choose the investigation depth according to the scope: a focused update can
follow the relevant domain; a broad reorganization can require the full memory
inventory. Follow cross-page references and investigate duplicates or
contradictions wherever they affect the result. Explain uncertainty and coverage
limits. Ask for a decision when the available context cannot support a sound
consolidation, preserving unresolved inputs for a suitable round. Never present partial analysis
as exhaustive.

The categories in the prepared `gpm.json` are the allowed destination choices.
Use only those categories under `memories/`. If none fits, discuss a configuration
change with the person before proceeding. Directory structure within a category,
page names and Markdown organization are editorial decisions.

## 3. Discuss the global plan

Show topic groups, proposal paths and interpreted titles, destination pages and
suggested changes. Several proposals can contribute to one memory; one proposal
can contribute to several memories. Suggested actions and destinations in the
inputs are hints: choose the destination that best fits the knowledge.

Account for every selected proposal with one or more destinations or a free-form
justification for excluding it from memory. Explain duplicates, incorrect
interpretations and unsuitable knowledge. Let the person merge or split groups,
change destinations and resolve contradictions. Update the global plan in
conversation; do not create a separate plan file.

Reorganize only memories related to the selected scope, as approved in the plan;
preserve knowledge that remains valid. Leave unrelated improvements outside
this consolidation. Wait for explicit approval of the global plan before
editing any canonical file.

## 4. Write final memories

Write canonical knowledge in complete final pages as ordinary Markdown. Omit
proposal instructions about action and destination from the memory prose.
Choose subject-based names such as
`memories/<configured-category>/<subject>.md`. Organize headings, metadata and
links to help future readers; preserve useful existing information and update
metadata when the approved content requires it. Review broken links, accidental
conflict markers and unfinished text yourself. Remove pages only as approved.

After plan approval, the principal can delegate independent memory files.
Assign one owner per file in each round. Each owner receives the exact paths,
current page, relevant proposals, approved decisions and cross-page context.
Only the principal performs Git, proposal cleanup and publication. Wait for
every delegate to finish before reviewing the result.

## 5. Review the whole result

The principal rereads every prepared page and checks it against the approved
plan. Review duplicates, contradictions, references and omissions across the
resulting pages. Resolve inconsistencies between delegates before presenting
the files. Check that every destination uses a configured category.

If content materially changes the approved plan, return to discussion and
obtain the person's decision before writing that change.

Call `memory_promote_check` for the current changed paths and Git diff. This
operation observes files without changing them or the worktree index. Review
every addition, edit and deletion. An empty memory diff has no publication path:
report the outcome and leave the proposals pending.

## 6. Review with the person

Show a global summary and the complete diff for each changed memory. Give the
worktree path for optional VS Code editing; do not open the editor automatically.
Terminal and editor review the same final files.

Finish all agent writing before human review. Stop writing while the person is
editing. Wait until the person asks for adjustments or says editing is finished
in conversation. For requested adjustments, reread the current files and
preserve human edits outside the request. New delegate rounds start from those
files. Run check again after changes and show the updated diff before seeking
publication approval. Keep the global plan current.

## 7. Publish

Ask for explicit approval to publish the current files and diff. The approved
summary accounts for every selected proposal by path, with its destinations or
free-form exclusion justification.

Before submitting, reread the current changed files and obtain the current diff
to confirm it matches the review. If anything changed, return to human review
and obtain a new approval. Do not edit files after publication approval.

After explicit approval, call `memory_promote_submit` with `summary` and the
reviewed memory `paths`. This commits those paths, pushes the branch and opens
a PR into `main`. Return the commit and PR link. The person reviews and merges.

## 8. Complete after merge

The next `$promote` confirms and cleans a previously merged publication before
preparing new inputs. The person can also run `gpm promote --finish` or ask for
`memory_promote_finish` after merging.

Cleanup confirms merge and removes only selected proposal paths whose blobs
still match. New, changed and unselected proposals stay pending. Published local
reads then follow `main`. The publication record links the PR to the selected
paths and blobs for cleanup.

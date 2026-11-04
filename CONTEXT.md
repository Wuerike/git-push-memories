# Git Push Memories

Shared knowledge becomes reviewable, current memory through conversation,
ordinary Markdown files, and Git publication.

## Language

**Proposal**:
A regular Markdown file under `proposals/` on the `proposals` branch, pending
consolidation. Path and Git blob identify the input. The agent interprets the
subject, title, suggested action and destination from its free content. Capture
generates a unique path; people can also add Markdown files manually.

**Canonical memory**:
An ordinary Markdown page under `memories/`. The agent chooses destinations from
the categories in `gpm.json` and organizes the content to fit the subject.
Published reading uses configured categories on `main`.

**Published read**:
A text search or file read from an immutable Git revision in the published
checkout's history. Search returns file paths, line intervals and the revision.
The agent can read related files or successive line windows at that revision.

**Consolidation**:
The agent and person select proposals, investigate related knowledge and agree
on a global plan in conversation. Every selected proposal receives destinations
or a free-form exclusion justification. The agent reads selected inputs and
pages to be edited completely, and follows related knowledge as needed. The
person approves the plan before editing and current files and diff before
publication. Uncertainty and investigation limits are explicit.

**Promotion worktree**:
An editable checkout on a fresh `promote/<UTC>-<random>` branch from fetched
`origin/main`. Fixed proposal inputs live outside the worktree. Agents and the
person review the same files. The principal assigns one owner per delegated
file, reviews cross-page coherence and conducts publication. All agent writing
pauses during human editing; requested adjustments preserve human changes.

**Memory check**:
Read-only observation of added, edited and deleted Markdown memory paths. Git
produces the diff using a temporary index outside the worktree. The agent reviews
editorial quality, references, categories and agreement with the approved plan.

**Publication**:
One commit containing the reviewed memory paths, pushed as a pull request into
`main`. Its summary accounts for every selected proposal. The person reviews
and merges the PR; selected inputs remain pending until merge confirmation.

**Publication record**:
Local linkage between the PR, branch, and selected proposal paths and Git blobs.
It supports post-merge cleanup on the publishing machine. Prepared editing state
belongs to the current MCP process; files remain on disk for inspection.

**Completion**:
After confirmed GitHub merge and verification of the merge commit in
`origin/main`, remove only selected proposal paths whose blobs still match.
Preserve new, changed and unselected proposals and refresh published reads.
Completion runs through the next `$promote` in Codex or `/promote` in Claude Code,
through `memory_promote_finish`, or through `gpm promote --finish`.

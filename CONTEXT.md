# Git Push Memories

Proposed knowledge becomes reviewable, current memory through conversation,
ordinary Markdown files, and Git publication.

## Language

**Proposal**:
Free Markdown captured on the `proposals` branch for later discussion. A title
names the file; suggested actions, destinations, and relevant origin belong in
the text. A proposal is pending input, outside published memory searches.

**Canonical memory**:
A Markdown page under a configured category in `memories/`, with the existing
canonical heading IDs and format. Published memory is the version on `main`.

**Consolidation**:
The agent and person read every proposal in a fixed input set and every memory
in the selected base, agree on a global plan in conversation, and prepare final
pages. Proposals and pages can relate in both directions: many proposals can
feed one page, and one proposal can feed many pages. Every input has destinations
or a free-form exclusion justification. The person approves the plan before
editing starts and the current files and diff before publication.

**Promotion worktree**:
An editable checkout on a fresh `promote/<UTC>-<random>` branch based on fetched
`origin/main`. Fixed proposal inputs live outside the worktree. Agents and the
person review the same final files. The principal owns Git and publication,
assigns one owner per delegated file, and reviews coherence across pages.
All agent writing pauses during human editing; adjustments preserve current
human edits outside the requested change.

**Canonical check**:
Validation of current files against configured categories and the existing
memory format. It preserves retained section IDs, fills IDs on new headings,
and returns the current diff before human publication approval.

**Publication**:
One global commit containing only canonical changes, pushed as a pull request
into `main`. Its free-form summary accounts for every initial proposal. The
person reviews and merges the PR; inputs remain pending until merge confirmation.

**Publication record**:
Local linkage between the PR, branch, and the paths and Git blobs of its initial
proposals. It supports post-merge cleanup on the publishing machine. It stores
neither a global plan nor an editing session and does not resume interrupted
consolidation.

**Completion**:
After confirmed GitHub merge and verification that the merge commit is in
`origin/main`, remove only concluded proposal paths whose blobs still match.
Preserve new or changed proposals and refresh local published reads. Completion
runs through the next `/promote`, `memory_promote_finish`, or
`gpm promote --finish`; it does not run in a background service.

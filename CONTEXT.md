# Git Push Memories

This context turns proposed knowledge into reviewable, current memory while keeping Markdown and Git as the durable authority.

## Language

**Curation transaction**:
A bounded, explicitly confirmed set of independent proposal decisions published as one Git commit.
_Avoid_: Bulk edit, partial batch

**Transaction marker**:
Private content-free metadata that identifies a prepared curation commit, its exact inputs and outputs, and the Git index state needed for recovery.
_Avoid_: Receipt, transaction log

**Curation recovery**:
Deterministic reconciliation of an interrupted curation transaction to either its complete commit or its unchanged baseline.
_Avoid_: Retry commit, automatic merge

**Page rewrite**:
Full replacement of an existing canonical page, bound to the hash of the file the reviewer observed.
_Avoid_: Page edit, overwrite, section replace

**Folded proposal**:
A proposal consumed by a page rewrite in the same curation transaction, with no canonical change of its own.
_Avoid_: Merged proposal, duplicate, silent discard

**Release**:
The consolidation of every pending proposal into canonical memory on a dedicated branch, reviewed and merged as one pull request into `main`.
_Avoid_: Publish, sync, batch (a batch is one commit inside a release)

**Release marker**:
The `release.lock.json` file on the proposals branch that lists the proposals included in a release and doubles as its lock; it never enters the release branch.
_Avoid_: Lock file, state file

**Release finalization**:
Closing a merged release by bringing the new `main` into the proposals branch and removing the consumed proposals and the marker in one push.
_Avoid_: Cleanup, sync

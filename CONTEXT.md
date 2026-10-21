# Git Push Memories

This context turns proposed knowledge into reviewable, current memory while keeping Markdown and Git as the durable authority.

## Language

**Source reference**:
An immutable identity for one committed source blob, comprising repository lineage, revision, repository-relative path, and content hash.
_Avoid_: Source path, file link

**Reference integrity**:
The deterministic status of whether a source reference resolves to the identified committed bytes in an available local repository.
_Avoid_: Truth, support, validation

**Semantic support**:
A human-reviewed assessment of whether referenced source content supports, contradicts, or is unclear about proposed knowledge.
_Avoid_: Integrity, verification

**Local source binding**:
A transient association between a source repository identity and an available local checkout used to evaluate reference integrity.
_Avoid_: Source ownership, persisted repository path

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

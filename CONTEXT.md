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

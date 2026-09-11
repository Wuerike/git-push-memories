# Immutable source references with transient local bindings

Status: accepted

Date: 2026-09-11

Source evidence is stored as repository lineage derived from root commits, a full commit object ID, a canonical repository-relative path, and the SHA-256 of the exact Git blob. Local checkout roots are supplied explicitly to `memory_reference_snapshot` and bound only for the lifetime of that MCP process; they are never written to proposals or memory. Bounded UTF-8 source excerpts may be read transiently with `memory_reference_expand` for semantic review but are never persisted by GPM. This chooses portable committed evidence over worktree paths, remote URLs, or persisted machine-specific roots, while allowing deterministic offline verification without fetching or executing source code.

This extends the proposal schema from ADR 0004 and supersedes its use of source paths for newly captured evidence; the rest of ADR 0004 remains in force.

Proposal schema v2 stores structured source references; schema v1 remains readable and its source paths are always `legacy_unverified`. Reference integrity is reported as `verified`, `unavailable`, `mismatch`, or `legacy_unverified` and never implies semantic support, which remains an assisted human assessment. A process restart therefore makes a structured reference unavailable until an explicit local snapshot rebinds the same lineage, and forks sharing the same roots intentionally share repository identity. References remain auditable in proposal history through the curation commit trailer rather than changing canonical memory page syntax in this increment.

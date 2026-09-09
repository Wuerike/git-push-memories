# Markdown proposal inbox and configured memory categories

Status: accepted

Date: 2026-09-09

The memory repository owns its layout through a mandatory root `gpm.json`. Canonical memory is read recursively only from configured category roots, while capture creates reviewable Markdown proposals in a repository inbox. An accepted proposal is committed directly on exactly `main`; proposal branches, JSON payloads, apply receipts, and the proposal apply/publish CLI lifecycle are removed.

This supersedes ADR 0003 and the proposal lifecycle decisions in ADRs 0001 and 0002. Those documents and the Phase 2 reports remain historical records of the behavior they evaluated.

## Repository configuration

The root configuration uses this closed schema:

```json
{
  "schema_version": 1,
  "memory_categories": ["domains", "decision"]
}
```

`memory_categories` must be non-empty and contain unique lowercase kebab-case names. Retrieval considers only `memories/<configured-category>/**/*.md`; root Markdown, direct `memories/*.md`, unconfigured categories, and `proposals/` are not memory authority.

## Proposal inbox

One proposal contains a title, a suggested action (`create`, `update`, or `remove`), an optional target, self-contained knowledge, and canonical source paths. It is rendered as human-readable Markdown with versioned frontmatter at:

```text
proposals/<id>-<title-slug>.md
```

The Intake Guard approves the complete closed payload before inbox state, files, or Git objects are created. Capture then serializes writers for the same local repository, verifies that the checked-out branch is exactly `main`, and creates a path-scoped commit from the latest `HEAD`. Unrelated staged work is preserved. The product does not fetch, pull, push, or otherwise access remotes.

Concurrent sessions in one clone are serialized and produce distinct linear commits. Independent clones can still race at push time; the human resolves that through the normal Git workflow.

## Consequences

- `/remember` ends after returning the proposal ID, Markdown path, and local commit.
- Pushing `main` remains an explicit human action.
- Proposals are reviewable with ordinary Markdown and Git tools; no product-specific show/apply command is required.
- Canonical memory is unchanged during capture.
- Curation is a separate future flow. When it consumes proposals, canonical memory updates and removal of the consumed proposal files must occur in the same commit.

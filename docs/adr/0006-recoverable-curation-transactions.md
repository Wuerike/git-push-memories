# Recoverable curation transactions use Git plus private markers

Status: accepted

Date: 2026-09-11

A curation transaction may contain one to sixteen confirmed decisions with distinct proposal IDs and canonical target paths. GPM builds one commit from the shared parent, preserves unrelated index entries, and records a private `0600` marker containing only identities, paths and hashes before changing the worktree or `main`. The marker in the private state root and a reconciled-index artifact beside the Git index make interruptions observable without introducing another authority: Git and Markdown remain canonical, while recovery either restores the exact parent baseline before `update-ref` or completes the already-published commit after it.

The Git index lock is owned only when it is the same file as the marker's hashed index artifact. Recovery never removes or replaces a different lock. Repeated targets are rejected because composing several semantic edits over one page would require ordered virtual baselines that the current preview contract does not expose; such decisions must be split into separate confirmed transactions.

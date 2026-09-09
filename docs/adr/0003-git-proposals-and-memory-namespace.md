# Git proposals and memory namespace

Status: accepted

Memory pages are direct `memories/*.md` files, and retrieval ignores Markdown outside that namespace. Each accepted proposal payload is committed as `proposals/<id>.json` on its own `gpm/proposal/<id>` branch without changing the current branch, index, or worktree; locks and terminal receipts remain private local state. This supersedes the proposal-persistence and root-page decisions in ADRs 0001 and 0002 because proposals must survive local state cleanup and be pushable for team review, while repository metadata such as `README.md` must not break retrieval.

## Consequences

- Creating a proposal writes Git objects and one dedicated local branch, but does not change the current checkout.
- Proposal branches are retained after apply or rejection as durable history and may be pushed explicitly; the product still does not access remotes.
- The Intake Guard runs before any proposal Git object is created, because pushed Git history cannot reliably erase sensitive content.
- New pages use `memories/<lowercase-kebab-case>.md`; root Markdown, `proposals/`, and nested directories under `memories/` are outside retrieval authority.

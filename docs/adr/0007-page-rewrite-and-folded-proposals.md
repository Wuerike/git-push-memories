# Page rewrite and folded proposals consolidate pages inside one transaction

Status: accepted

Date: 2026-10-20

An experiment on 2026-10-20 showed canonical pages accumulating sections: each curated proposal appended or replaced one section, and ADR 0006 rejects repeated targets in a transaction ("composing several semantic edits over one page would require ordered virtual baselines that the current preview contract does not expose"). Two proposals about the same page therefore could not be consolidated into one coherent page without separate transactions, each leaving an intermediate page behind.

`/curate` gains two decisions. `rewrite_page` replaces a whole existing canonical page and is bound to the hash of the file the reviewer observed. `fold_proposal` consumes an extra proposal inside the same transaction without any canonical change of its own; it only exists in a batch and must point, through `fold_into`, to a `rewrite_page` or to a `create_page` that carries a `replacement`, in the same batch. `create_page` gains an optional complete-page `replacement` so that a new page built from two or more proposals is created in a single run, without an intermediate page named after one person. The server preserves the baseline frontmatter on a rewrite. All three use the existing single commit and the existing transaction marker, and no new recovery item type is introduced. This does not contradict ADR 0006: each page still receives exactly one semantic edit, so there are no ordered virtual baselines, and folded proposals add no second target; repeated targets remain rejected except through fold.

Rejected alternatives: two sequential edits to the same page, which needs the virtual baselines ADR 0006 declined to expose, and `replace_section` on the H1, which is not a leaf section and fails with `ErrSectionNotLeaf`.

Confirmed decisions:

- A1: The server generates IDs for new headings in a rewrite (`<pageID>-<slug>`, the scheme used by `renderSection`). An explicit marker is accepted only if it already exists in the baseline.
- A2: `fold_proposal` exists only in batch preview (`memory_curate_batch_preview`). The individual preview rejects it, preserving its contract.
- A3: The server does not verify fact loss. Protection is the full diff plus the skill ledger, under human confirmation. Revisit only if losses go unnoticed in use.
- A4: `memory_page_read` returns the whole page or the error `budget_too_small`, never a truncated page. The skill requests `budget_bytes: 65536`, since rewriting from a truncated page would lose facts.
- A5: Grouping inbox proposals is agent judgment with human approval; the server computes no similarity. Add ranking only if proposed groups prove poor.
- A6: `rewrite_page` and `fold_proposal` accept proposals of any suggested action (`create`, `update`, `remove`). A `remove` may be absorbed as removal of a passage. A rewrite never deletes the whole page; that remains `remove_page`.
- A9: `create_page` accepts an optional complete-page `replacement` (one H1 plus sections, no frontmatter, no `@id` markers; the server generates every ID). A `fold_proposal` may target a `create_page` with `replacement` or a `rewrite_page`, always in the same batch.
- A10: A rewrite preserves the baseline frontmatter byte for byte; the replacement may omit it.
- A7: While curating proposals, the model may restructure existing pages when new information changes what they say, always with a diff and iteration. There is no proposal-less `/curate`; to tidy a page alone, use `/remember "restructure ..."` and the resulting `update` proposal becomes the `rewrite_page`.
- A8: A new preview for a proposal invalidates earlier tokens containing it, so a token from a superseded review round can never be applied.

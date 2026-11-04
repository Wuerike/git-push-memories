# Agentic memory verification

**Verdict**: PASS — both findings closed in the uncommitted working tree.
**Mutations**: 3 killed — R2, R6 and R7; see mutation-results.json. The author ran the independent candidates after the review and restored each file byte for byte.
**Proofs**: `proofs.md`, current working tree based on `79c025177b967641cd2796dd81c7230e21f4cd88`.
**Verifier**: independent sub-agent (author != verifier).
**Scope**: current uncommitted tree; commits and HEAD/clean-tree gates are prohibited by the user. The author may record closure using a working-tree fix and its passing check instead of a commit.

## Findings

| Finding | Evidence | Fix | Closed |
| --- | --- | --- | --- |
| F1 — R1 publication can normalize reviewed Markdown bytes through Git configuration. | A throwaway `TestVerifierCRLFPublicationWithGitTextNormalization` set `core.autocrlf=input` in a prepared worktree, wrote `Plain knowledge.\r\nAnother fact.\r\n` to a new selected Markdown path and submitted it. Reading the resulting commit blob returned `Plain knowledge.\nAnother fact.\n`. `changes` stages through `git add`, and publication stages through `Workspace.Commit`, so ordinary Git normalization affects the reviewed/published blob. | Stage raw file bytes without clean filters or line-ending conversion in review and publication. Add a publication regression that compares the actual committed blob with CRLF input under Git normalization configuration; include `.gitattributes` coverage if the implementation handles attributes separately. Preserve unrelated worktree staging. | Working-tree fix: Workspace.StageFiles and CommitFiles stage raw blobs through an isolated index. TestSubmitPreservesRawMarkdownUnderGitConversionAndCleanFilters checks autocrlf and attributes/clean filters, actual commit bytes and unrelated human staging. Full normal/race suites, vet and build pass. |
| F2 — linked research describes retired product contracts as current behavior. | `README.md` links `docs/research/memory-generation-mechanisms-comparison.md`. Its executive summary (lines 8–10), product section (61–151) and conclusions (742–801) claim `memory_expand`, three-section investigation, five-field proposals, `source_paths`, an Intake Guard and absent consolidation. The document records its original analysis date/revision, but repeatedly calls these the current product and links current local files, including deleted `internal/intake/guard.go`. This conflicts with repository instructions that documentation describe current code. | Update the GPM-specific descriptions, examples, local source references and conclusions to the current file/revision/line tools, free Markdown proposals and agent-led promotion. External-project research need not be refreshed for this feature. | Working-tree fix: GPM-specific research description, pipeline, comparisons, conclusions and local source links match current code. README and bedrock research licensing reference updated. All local documentation links resolve. |

## Own examples

1. **R1 — configuration boundary**: `core.autocrlf=input` plus a new CRLF memory file. The selected-path publication succeeded, but its commit blob differed from the reviewed file. This exposed F1. The original CRLF check (`TestCheckObservesOrdinaryMarkdownWithoutChangingFilesOrIndex`) asserts unchanged worktree bytes and index, which cannot establish byte preservation in the published blob.
2. **R6/R7 — filename and mode boundary**: an executable regular Markdown proposal named `proposals/topic [v2] literal*.md`, alongside `proposals/topic v literal-extra.md`. Preparing only the literal path returned exactly that proposal. After publication and confirmed squash merge, cleanup removed exactly the selected path and retained the other proposal. The probe passed.

Both examples used temporary Go tests in `internal/consolidation/verifier_probe_test.go`, ran with `go test -count=1 ./internal/consolidation -run '^TestVerifier' -v`, and were removed after execution. No production/test changes or commits were retained by the verifier.

## Mutation candidates

`mutations.json` contains three narrow, unique-snippet mutations with existing focused proofs:

- R2: invert the explicit-revision ancestry guard; `TestReadRejectsUnpublishedRevision` should reject the mutant.
- R7: stop preserving a proposal whose current nonempty blob differs; `TestFinishAfterSquashPreservesNewAndChangedProposals` should reject the mutant.
- R6: let unrelated proposals through an explicit preparation selection; `TestPrepareSelectsManualProposalPathsWithoutInterpretingContent` should reject the mutant.

The author executed all three candidates; every focused test failed on its mutant and the original file was restored. Results are recorded in `mutation-results.json`. Final normal/race suites, vet and build pass on the restored working tree. HEAD/clean-tree gates do not apply because the user prohibits commits. The review found no additional integrity defect in exact path selection, regular-file filtering, published ancestry checks or cleanup identity comparisons.

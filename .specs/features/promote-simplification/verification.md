# promote-simplification verification

**Verdict**: PASS
**Mutations**: 3 - probed C15, C22, C26
**Proofs**: `proofs.md` at 1ac27b07ab1d48faa6975e8f025175466f9dda6d
**Verifier**: independent sub-agent (author != verifier)

## Findings

None - probed retained section IDs, canonical-only publication, content-matched cleanup, deletion-only publication and cleanup with no eligible deletions; reviewed the agent contract against the approved scope.

## Mutations injected

Executed with `source .local/promote-env.sh && python3 .codex/skills/tlc-spec-driven/scripts/check_proofs.py promote-simplification --mutations .specs/features/promote-simplification/mutations.json`.

| Mutation | Location | Proof | Killed |
| --- | --- | --- | --- |
| Change the recognized retained section ID to `<id>-changed` | `internal/memory/document.go:32` | `TestPrepareDocumentPreservesAndCompletesIDs` | yes |
| Commit `.` instead of only checked canonical paths | `internal/consolidation/submit.go:72` | `TestSubmitCommitsOnlyCanonicalChanges` | yes |
| Reverse the proposal blob comparison | `internal/consolidation/finish.go:100` | `TestFinishAfterSquashPreservesNewAndChangedProposals` | yes |

Final runner result: **37 pass, 0 fail; mutations 3 killed, 0 not; Tree clean**. The runner restores mutated sources byte for byte. The only pending paths were feature metadata under `.specs/`, which the runner intentionally excludes from its source-tree cleanliness check.

The first sandboxed invocation could not write `.git/tlc-mutation-journal.json`; calling `mutate.py` exposed the read-only filesystem error that `check_proofs.py` had omitted while reporting zero mutations. The authorized invocation above completed all three mutations. The zero-mutation report was not accepted as verification evidence.

## Independent executed examples

A throwaway `internal/consolidation/verifier_examples_test.go` added two examples at the reviewed HEAD. It was removed before running final proofs. Both used only isolated local Git fixtures and fake GitHub, with no remote services.

Command: `source .local/promote-env.sh && go test ./internal/consolidation -run '^TestVerifier(DeletionOnlyCanonicalCommit|FinishWithEveryInputChanged)$' -count=1 -v`.

| Example | Rule and assertions | Result |
| --- | --- | --- |
| `TestVerifierDeletionOnlyCanonicalCommit` | PROM-22: remove the only canonical page while retaining a local helper. Check returns the deleted page and deletion diff; publication produces exactly one commit whose only change is `D memories/domains/checkout.md`. | PASS (0.22s) |
| `TestVerifierFinishWithEveryInputChanged` | PROM-26/PROM-27: change every initial proposal before squash merge. Cleanup removes zero proposals, preserves all three, leaves the proposals ref unchanged, refreshes the read checkout to merged main and removes the completed publication record. | PASS (0.34s) |

Go result: `ok github.com/Wuerike/git-push-memories/internal/consolidation 0.572s`.

## Semantic and assertion review

Reviewed the approved specification, the Checks and build/mutation settings, the green proof report and the production/test diff `e3b4902..1ac27b0`. Did not read the author's Handoff. There are no binding captures.

The skill explicitly requires full proposal and memory reading, accounts for every initial proposal, permits many-to-many destinations, and gates canonical edits on plan approval. Delegation gives one owner per file, current content and approved decisions, then requires the principal's cross-page review. Human review suspends agent writes and check calls; adjustments start from current files and preserve unrelated human edits. Publication rereads current changes and returns to review if the approved diff changed. These clauses preserve the distinction between agent decisions and file/Git tools; the contract tests are not treated as automated LLM execution.

The inspected assertions compare concrete bytes, IDs, paths, revisions, PR arguments and remaining proposals, rather than mere presence or non-error outcomes for those fixed values. No assertion gap or scope contradiction requiring a finding was identified. No product changes or commits were made by this verifier.

# promote-simplification proofs

**HEAD**: 1ac27b0
**Tree**: clean
**Result**: 37 pass, 0 fail; mutations 3 killed, 0 not

| Check | Proof | Ran | Passed | Result |
| --- | --- | --- | --- | --- |
| C1 | `go test ./internal/consolidation -run '^TestPrepareStartsFromFetchedMain$' -count=1` | 1 | 1 | pass |
| C2 | `go test ./internal/consolidation -run '^TestPrepareCopiesEveryProposalFromPinnedRevision$' -count=1` | 1 | 1 | pass |
| C3 | `go test ./tests/claudeadapter -run '^TestPromoteContractRequiresFullMemoryRead$' -count=1` | 1 | 1 | pass |
| C4 | `go test ./internal/consolidation -run '^TestPrepareIncludesSeventeenProposals$' -count=1` | 1 | 1 | pass |
| C5 | `go test ./internal/consolidation -run '^TestPreparedInputsExcludeLaterProposals$' -count=1` | 1 | 1 | pass |
| C6 | `go test ./tests/claudeadapter -run '^TestPromoteContractPresentsGlobalGroups$' -count=1` | 1 | 1 | pass |
| C7 | `go test ./tests/claudeadapter -run '^TestPromoteContractAccountsForEveryProposal$' -count=1` | 1 | 1 | pass |
| C8 | `go test ./internal/mcpserver -run '^TestConsolidationSupportsManyToManyMapping$' -count=1` | 1 | 1 | pass |
| C9 | `go test ./tests/claudeadapter -run '^TestPromoteContractWaitsForPlanApproval$' -count=1` | 1 | 1 | pass |
| C10 | `go test ./tests/claudeadapter -run '^TestPromoteContractRevisesPlanInConversation$' -count=1` | 1 | 1 | pass |
| C11 | `go test ./tests/claudeadapter -run '^TestPromoteContractLimitsReorganization$' -count=1` | 1 | 1 | pass |
| C12 | `go test ./tests/claudeadapter -run '^TestPromoteContractWritesCanonicalKnowledge$' -count=1` | 1 | 1 | pass |
| C13 | `go test ./tests/claudeadapter -run '^TestPromoteContractDelegatesByMemory$' -count=1` | 1 | 1 | pass |
| C14 | `go test ./tests/claudeadapter -run '^TestPromoteContractReviewsWholeResult$' -count=1` | 1 | 1 | pass |
| C15 | `go test ./internal/memory -run '^TestPrepareDocumentPreservesAndCompletesIDs$' -count=1` | 1 | 1 | pass |
| C16 | `go test ./internal/consolidation -run '^TestCheckAcceptsOnlyCanonicalDocuments$' -count=1` | 1 | 1 | pass |
| C17 | `go test ./tests/claudeadapter -run '^TestPromoteContractReviewsCurrentFilesAndDiff$' -count=1` | 1 | 1 | pass |
| C18 | `go test ./tests/claudeadapter -run '^TestPromoteContractPausesForHumanEditing$' -count=1` | 1 | 1 | pass |
| C19 | `go test ./internal/consolidation -run '^TestSubmitPreservesHumanEdits$' -count=1` | 1 | 1 | pass |
| C20 | `go test ./tests/claudeadapter -run '^TestPromoteContractDiscussesMaterialPlanChanges$' -count=1` | 1 | 1 | pass |
| C21 | `go test ./tests/claudeadapter -run '^TestPromoteContractRequiresApprovalOfCurrentResult$' -count=1` | 1 | 1 | pass |
| C22 | `go test ./internal/consolidation -run '^TestSubmitCommitsOnlyCanonicalChanges$' -count=1` | 1 | 1 | pass |
| C23 | `go test ./internal/consolidation -run '^TestSubmitPushesBranchAndCreatesMainPR$' -count=1` | 1 | 1 | pass |
| C24 | `go test ./internal/consolidation -run '^TestSubmitRecordsFreeFormProposalSummary$' -count=1` | 1 | 1 | pass |
| C25 | `go test ./internal/consolidation -run '^TestFinishKeepsProposalsUntilConfirmedMerge$' -count=1` | 1 | 1 | pass |
| C26 | `go test ./internal/consolidation -run '^TestFinishAfterSquashPreservesNewAndChangedProposals$' -count=1` | 1 | 1 | pass |
| C27 | `go test ./internal/consolidation -run '^TestFinishRefreshesPublishedReads$' -count=1` | 1 | 1 | pass |
| C28 | `go test ./internal/mcpserver -run '^TestPromotionToolContractUsesFiles$' -count=1` | 1 | 1 | pass |
| C29 | `go test ./internal/consolidation -run '^TestPrepareAndSubmitDoNotUseSharedReleaseMarker$' -count=1` | 1 | 1 | pass |
| C30 | `go test ./internal/mcpserver -run '^TestConsolidationPreservesRememberAndPublishedReads$' -count=1` | 1 | 1 | pass |
| C31 | `go test ./tests/claudeadapter -run '^TestPromotionDocumentationDescribesImplementedWorkflow$' -count=1` | 1 | 1 | pass |
| C32 | `go test ./internal/consolidation -run '^(TestConsolidationFailureStopsWithoutDeletingWork|TestSubmitFailureKee` | 3 | 3 | pass |
| C33 | `go test ./tests/claudeadapter -run '^TestPromoteContractStopsOnIncompleteContext$' -count=1` | 1 | 1 | pass |
| C34 | `go test ./internal/consolidation -run '^TestPrepareWithoutProposalsDoesNotPublish$' -count=1` | 1 | 1 | pass |
| Build | `go vet ./...` | - | - | pass |
| Build | `go build ./...` | - | - | pass |
| Build | `go test -race ./...` | - | - | pass |

## Mutations

| Mutation | Location | Killed |
| --- | --- | --- |
| C15: reject retained IDs by changing their recognized value | `internal/memory/document.go:32` | yes |
| C22: commit the whole worktree including helpers and proposals | `internal/consolidation/submit.go:72` | yes |
| C26: invert blob match and remove changed inputs | `internal/consolidation/finish.go:100` | yes |

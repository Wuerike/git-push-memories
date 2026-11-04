# Working-tree verification

**Status**: PASS
**Base HEAD**: 79c025177b967641cd2796dd81c7230e21f4cd88
**Scope**: current uncommitted working tree. The user prohibits commits; proof-at-HEAD and clean-tree gates do not describe this delivery.

## Checks

- R1/R4: `go test -count=1 ./internal/consolidation` is covered by the full suite. Plain text, multiple H1s, changed frontmatter, CRLF and pages over 128 KiB remain unchanged; additions, human edits and deletions have Git diffs and leave the worktree index intact. The selected literal wildcard filename test proves exact publication path selection.
- R2: `go test -count=1 ./internal/retrieval ./internal/workspace` is covered by the full suite. Excerpts carry paths/lines/revisions, line windows preserve bytes, old revisions survive publication and restarts, unpublished revisions and path escapes are rejected, and root replacement does not retarget reads.
- R3/R5: `go test -count=1 ./tests/claudeadapter` passes. Skill instruction review covers configured category choices, adaptive investigation, full selected-input and edited-page reading, explicit uncertainty, plan approval, current-diff approval and human-edit preservation. These checks prove shipped instructions, not LLM judgment.
- R6/R7: `go test -count=1 ./internal/inbox ./internal/consolidation ./internal/mcpserver ./tests/e2e` is covered by the full suite. Manual proposals, selected proposal and memory paths, pending unrelated inputs, path/blob cleanup and existing concurrency/merge safeguards pass. The E2E executes the real binary against Git repositories and a fake GitHub CLI.
- Full suite: `GOTOOLCHAIN=go1.25.12 GOPROXY=off go test -count=1 ./...` passes; log `/tmp/gpm-agentic-all.log`.
- Race detector: `CGO_ENABLED=1 CC='/tmp/promote-tools/zig/zig cc' go test -race -count=1 ./...` passes; log `/tmp/gpm-agentic-race.log`.
- `go vet ./...`, `go build ./...` and `git diff --check` pass.
- After simplifying the capture response back to its existing GitHub operation, targeted inbox/MCP/E2E tests pass; log `/tmp/gpm-agentic-capture.log`. Targeted race/vet/build verification also passes; race log `/tmp/gpm-agentic-capture-race.log`.

## Independent review

Verifier dispatch: current uncommitted tree, no code changes or commits authorized for the verifier. Review findings and three proposed mutations are recorded separately.

## Final closure

- Independent findings F1 and F2 are closed in `verification.md` without commits.
- Publication uses exact blobs, including CRLF under `core.autocrlf=input` and `.gitattributes` clean filters. Unrelated staged paths remain staged and outside the published commit. Deletion and directory-containment regressions also pass.
- All three independent mutation candidates are killed; `mutation-results.json` records test failures and every mutated source is restored.
- Final full normal suite passes: `/tmp/gpm-agentic-final-tests.log`.
- Final full race suite passes: `/tmp/gpm-agentic-final-race.log`.
- Final `go vet ./...`, `go build ./...`, production deadcode analysis, gofmt and `git diff --check` pass. Production deadcode analysis has no findings.
- Local links across README, product docs and research docs resolve.
- HEAD remains `79c025177b967641cd2796dd81c7230e21f4cd88`; the repository index is unchanged. No commits were created.

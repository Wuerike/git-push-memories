# Agentic memory

**Approved**: 2026-11-03 - user: "Concordo com todos, com uma pequena nota para o item 3, estão todos aceitos, aplique as alterações, delete códigos que não serão mais utilizados, atualize docs, mas não comite nada"

## Requirements

- R1: When publishing memory, GPM SHALL preserve ordinary Markdown bytes without interpreting headings, IDs or frontmatter.
- R2: When searching published memory, GPM SHALL return text excerpts with a Git revision, path and line interval; reading that revision SHALL return the selected file or lines independently of later updates.
- R3: GPM SHALL expose the categories in gpm.json as the allowed category choices to the agent. Skills SHALL choose destinations only in those categories.
- R4: When checking a promotion, GPM SHALL return Git-generated differences including additions, edits and deletions without modifying the worktree or its index.
- R5: Skills SHALL investigate relevant knowledge adaptively, read selected proposals and edited pages completely, and disclose uncertainty while preserving human plan and publication approval.
- R6: GPM SHALL list regular Markdown proposals under proposals/ using path and blob identity and prepare all or explicitly selected proposal paths. Titles and grouping SHALL be interpreted by the agent.
- R7: GPM SHALL preserve Git concurrency checks, local edits, published-only reading and cleanup of unchanged selected proposals after confirmed merge.

## Execution

1. Plain-file retrieval and immutable revision reading: internal/memory, internal/retrieval and MCP reading tools.
2. Proposal path/blob listing and optional selection: inbox and consolidation preparation/cleanup.
3. Read-only Git diff and publication: consolidation and workspace Git adapter.
4. Adaptive skills, public docs and obsolete dependency/code removal.
5. Integration tests, full verification and independent review.

## User constraints

Work on the current tree without commits. Existing tests for retired format/handle/source contracts are replaced by tests of the accepted behavior. HEAD/clean-tree verification gates are inapplicable; run checks on the working tree and record their actual results.

## Checks

- R1/R4: go test ./internal/consolidation
- R2: go test ./internal/retrieval ./internal/mcpserver
- R3/R5: skill contract review and go test ./tests/claudeadapter
- R6/R7: go test ./internal/inbox ./internal/consolidation ./tests/e2e
- Build: make verify-offline

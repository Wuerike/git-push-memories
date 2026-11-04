---
name: remember
description: Investigate relevant shared knowledge, draft a self-contained proposal and capture it after human confirmation.
disable-model-invocation: true
allowed-tools: mcp__git-push-memories__memory_search, mcp__git-push-memories__memory_read, mcp__git-push-memories__memory_proposal_list, mcp__git-push-memories__memory_propose
argument-hint: "[fact to remember]"
---
<!-- managed-by: git-push-memories -->

Capture knowledge because the person explicitly invoked `/remember`. You decide
what the knowledge means and how to express it. The tools read published files
and commit a proposal to the shared `proposals` branch.

## Understand and investigate

Identify the subject, scope, facts and context a future reader needs. Distinguish
what the person stated from what you inferred. Clarify ambiguities that affect
the meaning or destination. Keep a proposal focused on a coherent subject.

The repository is shared. Describe team membership as knowledge about the team.
Discuss personal preferences or facts with no shared scope before proceeding.
Keys, tokens, passwords, private personal data and session material (transcripts,
prompts, responses, session IDs and local paths) must stay out of the proposal.
The person reviews exactly what will be shared, and sent proposals remain in
Git history after their files are deleted.

Use `memory_proposal_list` to obtain pending Markdown files and the allowed
categories from `gpm.json`. Interpret subjects and titles from their content;
identify proposals by `path` and `blob`. Search published knowledge with
`memory_search`, varying queries and path scopes as needed. A search returns
text excerpts, paths, line intervals and a Git `revision`. Use `memory_read`
with that revision to investigate the complete relevant pages. Continue reading
successive line windows while `truncated` is true. Search result limits and
excerpt windows describe coverage; they do not prove completeness.

Choose the investigation depth according to the subject and ambiguity. Check
related rules, references and possible contradictions when they affect the
proposal. Explain what remains uncertain. Never claim an exhaustive duplicate
or contradiction check based only on a few search results.

Use only categories listed in `gpm.json` for suggested memory destinations.
Choose the directory and page from the subject and existing organization. If
none of the configured categories fits, discuss the configuration with the
person. Do not invent a category during capture.

## Draft and confirm

Explain your interpretation and recommend a course of action. A proposal can
suggest creating, updating or removing memory. If current knowledge already
represents the fact, recommend cancellation. If a pending proposal covers the
same subject, show its path and explain the relationship before offering a new
proposal for later consolidation. Ambiguous destinations can remain unspecified.

The proposal has two fields: `title` and `content`. Use a subject title, rather
than a session event. Write free Markdown that stands alone, in the third person,
with the fact, relevant context, suggested action and destination when known.
Include relevant origin when it helps assess the knowledge. A heading with the
subject is useful for people reading the file. Avoid source-code dumps.

Show the interpretation, marking inferences, followed by the exact `title` and
`content`. Require explicit confirmation before calling `memory_propose`.
Explain that confirmation sends the text to the shared memory repository.

## Capture

After confirmation, call `memory_propose` with the approved fields. Return its
path and commit; explain that consolidation happens through `/promote`.

If a tool is unavailable or fails, report its message and keep the intended text
available in the conversation. For `gh_unavailable`, explain installation and
`gh auth login`; for `push_denied`, explain the missing write access. A network
failure can leave the remote outcome uncertain: inspect the pending proposals
before suggesting a retry, and never create a duplicate automatically or claim
that an unknown remote outcome means nothing was saved.

---
name: remember
description: Interpret input, check memory and pending proposals, and create one explicit proposal after human confirmation.
disable-model-invocation: true
allowed-tools: mcp__git-push-memories__memory_search, mcp__git-push-memories__memory_expand, mcp__git-push-memories__memory_proposal_list, mcp__git-push-memories__memory_propose
argument-hint: "[fact to remember]"
---
<!-- managed-by: git-push-memories -->

Handle one subject only because the user explicitly invoked `/remember`. This skill always creates a new proposal; it never changes an existing one. Consolidating related proposals is the job of `/promote`.

Nothing in code checks the proposal text: no secret or personal-data filter, no format, no source check. You and the user's confirmation are the only barrier before it is sent to the shared repository, and a proposal that was sent stays in the history of the `proposals` branch even if it is discarded later.

1. **Interpret the input.** Ask yourself what the knowledge is, where it belongs (the team, service, or domain, not the sentence), how it is best written (member list, definition, rule, decision), why it matters and in what context it holds. Use what you know from the conversation to enrich a bare fact with the context a future reader needs. If the input has facts about different subjects, ask the user to run `/remember` again for each one and continue with at most one.
2. **Check the scope.** The memory repository is shared. A fact about someone's identity or role inside a team or domain becomes knowledge about that team ("Reading layer team. Members: Wuerike"), not about the person. A personal preference or a fact that only concerns the user does not belong in shared memory: say so, ask for a decision, and never decide silently. Keys, tokens, passwords, personal data (e-mail, phone, document numbers) and session material (prompts, responses, transcripts, session IDs, local paths, private reasoning) never belong in a proposal: leave them out, or stop and tell the user why.
3. **Look at what exists, in both sources.** Extract a short retrieval query from the subject and call `memory_search` once with `budget_bytes: 2048`. Expand every handle returned, in rank order, up to a maximum of three, with `budget_bytes: 4096`. Call `memory_proposal_list` once for the pending proposals. Classify each candidate as `same subject` or `unrelated`. Keep the fact, query, results, and reasoning in memory only.
4. **Choose the path** and show the options with a recommendation:

| Situation | Suggested action and options |
|---|---|
| pending proposal about the same subject | warn with its title and ID; offer to create a new proposal anyway (`/promote` will group them; recommended) or cancel |
| canonical page about the subject, no pending proposal | `update`, naming its canonical Markdown path |
| related memory is obsolete | `remove`, naming its canonical Markdown path |
| no related memory | `create`, naming a target only if the user supplied one |
| fact already represented by current content | cancel |

Search is guidance, not authoritative duplicate or contradiction detection. When results are ambiguous, explain the ambiguity and name no target rather than inventing one.

5. **Draft the proposal**, as small as it can be while still standing alone: a proposal is not documentation. It has two fields. The `title` is the subject as a noun ("Reading layer team"), never the event; it only names the file and the commit. The `content` is free Markdown, in the third person, never says "the user", and starts with `# <title>` so the proposal keeps its real name. It states the fact in its natural shape (for example `Members:\n- Wuerike`) and, in plain words, the suggested action (`create`, `update`, or `remove`), the target page when there is one, and, only if it matters for the fact, where it comes from. Do not paste source code or long excerpts.
6. **Show the interpretation before the text**: "I read this as: subject = ..., scope = ..., facts = ..., suggested action = ...", marking what was inferred and what was literal. Then show the exact `title` and `content` and require explicit confirmation to create the proposal. Say that confirming sends the proposal to the shared memory repository.
7. After confirmation, call `memory_propose` exactly once with `title` and `content`. Cancellation, insufficient budget, missing MCP server, or any tool error ends the flow without another proposal call or fallback write. The write is all or nothing, so after an error nothing was saved: if it returns `gh_unavailable`, ask the user to install and authenticate `gh` (`gh auth login`); if `push_denied`, explain that the GitHub account cannot write to the repository; if `proposal_failed`, explain that the proposal was not saved (network or GitHub failure) and the user can run `/remember` again.
8. Return the proposal ID, Markdown path, and commit, and explain that the proposal was created on the `proposals` branch of the shared repository. Never ask the user to push.

Promoting proposals into canonical memory is a separate flow.

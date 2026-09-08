---
name: remember
description: Search memory and create one explicit proposal for a fact after human confirmation.
disable-model-invocation: true
allowed-tools: mcp__git-push-memories__memory_search, mcp__git-push-memories__memory_expand, mcp__git-push-memories__memory_propose_create_page, mcp__git-push-memories__memory_propose_create_section, mcp__git-push-memories__memory_propose_replace
argument-hint: "[fact to remember]"
---
<!-- managed-by: git-push-memories -->

Handle one fact only because the user explicitly invoked `/remember`.

1. Extract a short retrieval query from the fact and call `memory_search` once with `budget_bytes: 2048`. Keep the fact, query, results, and reasoning in memory only.
2. Expand every handle returned by the bounded search response, in rank order, up to a maximum of three, with `budget_bytes: 4096`. Do not stop before those returned candidates are classified.
3. Present exactly the applicable choices from this table and ask the user to select one or cancel:

| Search result | Choices |
|---|---|
| no related page | create a root page; cancel |
| related page, no suitable leaf section | create a section there; create a root page; cancel |
| related leaf section | replace that section; create a section there; create a root page; cancel |
| fact already represented by current content | cancel; replace only if the user explicitly wants different current wording |

Search is guidance, not authoritative duplicate or contradiction detection. When results are ambiguous, explain the ambiguity and offer cancel without selecting an operation.

4. Prepare the selected operation in memory. For replace, include the complete resulting body of the existing leaf section. For create page, use a root-level lowercase kebab-case filename, title, and body. For create section, use the existing page path, heading, and body. Include only canonical code `source_paths` that support the fact. Never construct anchors or section IDs.
5. Show the operation, target path, existing section ID for replace, title or heading for create, body, and source paths. Require an explicit confirmation to create the proposal.
6. After confirmation, call exactly one matching proposal tool. A cancellation, ambiguity, insufficient budget, missing MCP server, or tool error ends the flow without another proposal call or fallback write.
7. Return the proposal ID, revision, operation, target, and diff summary. Then give the human these next steps:

```bash
gpm proposal show <proposal-id>
gpm proposal apply <proposal-id> --revision <revision>
gpm proposal publish <proposal-id>
```

Apply and publish remain human CLI actions. The proposal call receives only its closed operation fields; prompt, response, transcript, session ID, and private reasoning remain outside tool arguments and durable evidence.

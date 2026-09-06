Inspect this repository and current and retired priority parcel escalation policy. Determine the current timeout, queue, and queue owner from direct repository evidence.

Then call `memory_propose_replace` exactly once with:

- target path `domains/parcel-dispatch.md`;
- section ID `parcel-dispatch-priority-escalation`;
- a concise replacement that records only the current policy;
- the repository paths that directly support the current timeout, queue, and owner.

Use exactly this replacement template, substituting only values established by current evidence:

```text
- Priority parcels escalate after <seconds> seconds through `<queue>`.
- The active queue is owned by `<owner>`.
```

The replacement value must begin with one newline before the first bullet and end with one newline after the second bullet, preserving the blank line after the section anchor.

Do not edit the code repository. Do not call apply or publish. Do not include this instruction or any session content in the proposal.

# Survivor review format

A review must make sense without the source code, mutation hashes, or earlier chat. Give each decision the context of a
conversational explanation, not a terse audit row. A table may summarize the queue, but does not replace the decision
entries below.

## Categories

Group related entries under these headings, omitting empty groups:

| Category                  | What belongs here                                              | Default next step                                              |
| ------------------------- | -------------------------------------------------------------- | -------------------------------------------------------------- |
| Behavior to pin           | Intended observable behavior needs stronger tests              | Judge recommends behavior and an independent test plan         |
| Equivalent output         | Applied evidence supports unchanged promised results           | Recommend a specific exception, with scope and limits          |
| Presentation variation    | Wording or formatting changes without losing required meaning  | Explain why the changed result remains useful                  |
| Supported-input boundary  | Differences occur only outside the current contract            | State the excluded input and check the validation boundary     |
| Product tradeoff          | Plausible behaviors serve different goals or change ownership  | Judge makes a bounded call or escalates with alternatives      |
| Requirement conflict      | Accepted rules disagree or implementation stages differ        | Name the conflict; do not silently resolve protected rules     |
| Engineering investigation | Evidence, reachability, identity, or a valid oracle is missing | Continue investigation without requesting speculative approval |

Classification and readiness are separate. An unprobed equivalence candidate belongs in engineering investigation until
the required evidence exists. A presentation change that destroys useful meaning is not harmless variation. Do not treat
an excluded input as universally equivalent or claim a validator rejects it without checking the actual path.

## Batch summary

State the source revision and whether reports are fresh or historical. Explain that mutation IDs identify temporary
changed code, not commits. Distinguish the base commit from any later fix commits; never imply one commit per survivor.

Summarize the categories, judge recommendations, evidence readiness, and any actual human decisions needed. Name counts
only when the inventory supports them. Say whether tests, implementation, or accepted lists changed.

## Decision entry

Use a stable readable label such as `R2.1`, followed by a plain-language title. One entry may group mutations only when
they share the same behavior, scope, evidence standard, and decision. Keep an exact membership list for traceability.

Each entry contains:

1. **Category and status.** Separate the product decision, test planned or verified, implementation, exception pending,
   and human approval. State what action is requested now.
1. **What this part does.** Explain the component's job, who experiences the result, and what the component does not do.
   Distinguish command-list entries from shell arguments, suggested values from answers, and allocation estimates from
   limits when those distinctions matter.
1. **What changed.** Describe the actual temporary change in ordinary language. A small before/after code fragment is
   optional support, not the explanation.
1. **Concrete example.** Give readable inputs, expected output, and the changed output or observed lack of difference.
   Mark a predicted result as predicted. Explain consequences such as lost answers, extra questions, or changed
   commands. For identical results, state what still happens and what may differ, such as allocation.
1. **Judge's recommendation.** State the proposed behavior, governing principles or accepted requirement, assumptions,
   and countervailing considerations. Explain why the recommendation follows; do not merely say it is equivalent or
   deterministic. Identify the limits of the judge's authority.
1. **Alternatives and consequences.** Give meaningful alternatives for a real behavior choice, including keeping the
   current rule when relevant. Explain what each changes for the people using the product. Do not invent alternatives
   for a mathematical fact just to fill the format.
1. **Evidence and test plan.** Say what was actually run, which input reached the mutation, what result was compared,
   and what remains unverified. For a behavior to pin, give an observable acceptance criterion, independent oracle, and
   wrong behavior the test must reject. Link exact evidence and contract rules. Runtime-dependent equivalence must name
   its dependency and limits.
1. **Decision scope.** State exactly what agreement would authorize and what remains unchanged. List mutation IDs and
   source locations last, as lookup references. Distinguish selecting behavior from approving protected edits or adding
   an exception.

Use only the detail that helps the person make the call, but do not omit an important distinction merely because the
mutation is a one-character change. Put dense logs and full ID inventories behind links.

## Checkboxes and approvals

Use an unchecked Markdown task item only for a choice or approval the person actually needs to make. Put the
recommendation in its text, for example:

- [ ] Approve preserving unrelated empty answer maps during this cleanup.

Follow it with the context, example, alternatives, evidence, and scope. A person may reply using stable entry labels
instead of editing hashes. Record explicit approval and its scope, not an agent's inference of agreement. Do not label a
delegated agent decision as human-approved.

No checkbox is needed for ordinary proof work, a missing test run, or a choice already settled within delegated
authority. List those as work with a status. An exception awaiting proof is not ready for acceptance. Never treat an
unchecked box as approval or ask someone to approve a whole historical list.

## Small example of the required explanation

A map reservation mutation changes an initial space estimate from addition to subtraction. Explain that the joiner
combines plan answers with saved personal answers, and that the personal answer wins when both contain the same key.

For two plan answers and three saved answers, the estimates become five and minus one. State the observed runtime
handling of the negative estimate and whether all answers were still copied. Explain that the map grows as entries are
inserted: subtraction changes the allocation estimate, not which answers are retained. If proven, recommend an
output-equivalence exception while keeping the sensible original code. State that allocation cost may differ.

This example is a writing standard, not evidence for any particular survivor. The actual entry still needs a baseline,
an applied mutation, a reaching input, and current identity verification.

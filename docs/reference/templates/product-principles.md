---
approval: person
reviewed: <YYYY-MM-DD>
---

# Product principles

<!-- Goes to docs/reference/product-principles.md and is edited in place. The behavior judge skill reads it and not the records. A person writes the first version before the first survivor review. The delegated authority is in the behavior judge skill. approval says who last approved a change, and reviewed is the date of the last full review. -->

## Context

<Who the product serves, and what the work costs them today. Name the questions that keep coming up and that the code alone cannot answer.>

## Goal

The goal is <who the product serves, and what it saves them>.

## Principles

Each principle has a short label so a decision can cite it. Write four to eight. Each says what to prefer and what it does not permit, and is about the people using the product. Each ends with a `Records:` line that lists the decision records it comes from, as four-digit numbers, or `none`. The records check fails when a listed record does not exist or is superseded, so a principle never rests on a dead record.

### P1. <Prepare what the operator reliably knows>

<What to prefer. What this does not permit.>

Records: <NNNN, NNNN>

### P2. <Respect ownership and deliberate choices>

<A default is a suggestion, not an answer. Only a person's explicit acceptance makes it one.>

Records: <NNNN, NNNN>

### P3. <Make the next useful action clear>

<A diagnostic should help a person understand the problem, not merely contain some text.>

Records: <NNNN, NNNN>

### P4. <Preserve work and keep changes local>

<Do not discard data or rewrite unrelated data as a side effect of another action. Prefer explicit, recoverable changes. When two options serve the person equally well, prefer fewer side effects.>

Records: <NNNN, NNNN>

### P5. <Match protection to the threat you have>

<Name the threats this stage faces. Keep cheap protection against them. Record each deferral in an ADR so it can return. An agent does not defer a protection without a person's approval.>

Records: <NNNN, NNNN>

### P6. <Ask for information precise enough for the decision>

<Prefer an observed event or a clearly labeled proxy to another question. Close enough is not permission to guess, silently accept a default, hide ambiguity, or relax safety or authorization.>

Records: <NNNN, NNNN>

These principles are not points in a score. Privacy, correctness, ownership, and explicit commitments constrain convenience. Within those constraints, reduce avoidable work and uncertainty, then prefer a smaller and more reversible change.

## Known tensions

- <Two principles that pull against each other, such as P2 and P3 on whether to offer a skip. The judge escalates these and does not decide them.>

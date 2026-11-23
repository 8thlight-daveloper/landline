---
approval: person
reviewed: 2026-11-23
---

# Coding principles

## Context

The product principles in [product-principles.md](product-principles.md) say what Landline does for the people who use
it. These principles say how its programs are built: how one program learns what another knows, and which of those
channels are contracts. A builder and a reviewer read them alongside the product principles.

## Principles

Each principle has a short label so a decision or a review can cite it. Each says what to prefer and what it does not
permit, and ends with a `Records:` line that lists the decision records it comes from, as four-digit numbers, or `none`.

### C1. Programs share facts through formats built for programs

A program never depends on text written for people, such as log lines, banners, help, or error messages. A person may
reword that text at any time, and nothing warns that code depended on it. When one program needs a fact another program
knows, they share it through a channel built for programs, with a stated format: an API response, a structured file, an
argument, or an environment variable. Pick the channel that fits who needs the fact and when they need it. Every such
format is a contract, and a change to it needs a record. New or changed code follows the principle, and code written
before it is tracked and fixed in its own change, which does not block unrelated work.

Records: 0220

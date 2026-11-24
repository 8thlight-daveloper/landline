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

### C2. Clean text that came from outside before a person sees it

A program never shows a person text it received from another program as it came. That covers text from the switchboard,
a message another agent wrote, and anything read over the network. Before such text reaches a terminal, a menu, a
notification, or a log a person reads, it passes through one shared cleaning step per module. That step removes control
characters, among them the escape character that starts terminal sequences. A new command uses the shared step and never
writes its own. Text that a program only stores or passes on, such as a message body the switchboard delivers, is kept
as it came, and the program that shows it cleans it.

Records: 0200

### C3. Remove a moving part before you guard one

When a fix needs a new special case, a value that stands for two states, or a check of one mechanism against another,
first look for a part to remove: a second owner of one resource, a second path that does the same job, or a state that
one value has to mean twice. Add a guard only when no part can go, and say in the change which part you kept and why. A
review finding at the edge of a fix from the round before is a sign that the fix added a part, and the next fix starts
by asking which part to take out.

Records: 0220, 0260

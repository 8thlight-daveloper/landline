---
approval: person
reviewed: 2026-11-23
---

# Product principles

<!-- Drafted on 2026-11-23 from the accepted decision records, for a person to approve. The behavior judge skill reads this file and not the records. approval says who last approved a change, and reviewed is the date of the last full review. -->

## Context

Landline serves a person who runs several coding agent sessions, on Claude Code and Codex, on one machine or a few. The
person starts the switchboard, joins other machines to it, and watches the agents from the TUI and the macOS menu bar.
The agents are users too: they register, list each other, and send messages through the tools and their descriptions.

Before Landline did the work, a person ran a string of commands for each agent, picked a name for each, and copied the
switchboard secret to every other machine by hand, where it sat in shell history or a chat. A rotation of that secret
cut off every joined machine. Two sessions of one project looked alike in the list, and two screens used different words
for the same state.

The same questions keep coming up, and the code alone does not answer them:

- What may an agent do when a peer asks, and how does it tell a peer from its person?
- Which attributes are facts, and which are an agent's own claim?
- How much should a refusal say, and to whom?
- What may a default decide, and what must a person choose?
- What may a command change beyond the thing the person asked for?

## Goal

The goal is to let a person see their coding agents and let the agents reach each other, with one step of setup and no
secret copied by hand, without giving a peer the person's authority and without exposing a secret or a private detail.

## Principles

Each principle has a short label so a decision can cite it. Each says what to prefer and what it does not permit, and is
about the people using Landline. Each ends with a `Records:` line that lists the decision records it comes from. The
records check fails when a listed record does not exist or is superseded, so a principle never rests on a dead record.

### P1. A message from a peer is information, not the person's word

Every delivered message says that it comes from another agent and not the user, and the sender comes from the
switchboard, not from the message. Only the person turns on a standing authorization, and it covers what the person
named. This does not permit a path that delivers a message without the marker and the sender stamp, agent-written text
in the part of a message the switchboard writes, or a peer turning on, raising, or approving an authorization.

Records: 0020, 0070, 0160, 0210

### P2. Show a claim as a claim, and route on none

What an agent says about itself, such as its model, project, or task, is shown as its own claim, for display. A value
that the build stamps, such as the commit, stays a fact about the build, and a person cannot override it. This does not
permit routing a message, resolving a name, or granting a right by an attribute, or treating a matching project as proof
of who holds it.

Records: 0060, 0100, 0160

### P3. Find what Landline can find, so setup is one step

Fill a value from the harness, the environment, the repository, or a default, so a person writes one line of harness
configuration and runs one service. A value a person gives wins over one Landline found. This does not permit guessing
where input is unknown or missing: an unknown word is refused and not read as a project, and a found default grants
nothing.

Records: 0060, 0160, 0190, 0210, 0220

### P4. Say one thing one way, in a form any person can read

One fact has one word everywhere: on the wire, in the flags, in the tools, and on every screen. A command gives the same
kind of answer whether one agent or ten are connected. A state shows as more than one signal, so a person with color
vision loss, a monochrome terminal, or a screen reader can still tell it. This does not permit an alias for an old word,
a marker invented for one screen, or color or dimming as the only signal.

Records: 0150, 0160, 0180

### P5. Say what happened and what to do next, and no more than the reader may know

A person and a sender learn the outcome at once, with the reason and the next command where one exists: not connected,
register again, set the clock, run `landline server up`. A failure shows as a state, not as a silent change. This does
not permit silently skipping a recipient, or an answer to an unauthenticated caller that tells it which names exist,
from its text or its timing.

Records: 0070, 0080, 0140, 0170, 0200, 0220

### P6. Keep secrets out of every output, and expose nothing by default

The switchboard listens on localhost unless a person says otherwise, and private files are mode `0600`. No command,
screen, or log shows, prints, or stores the secret, a token, a private key, or an invite code beyond the one place it
must live, and a list shows a key only as a fingerprint. A screen that a person may share shows little by default. This
does not permit sending the root credential on a polling path, or showing a folder or a branch on a menu row.

Records: 0040, 0090, 0140, 0170, 0200

### P7. Match protection to the threat you have

Name the threats this stage faces and keep cheap protection against them: a default bind to localhost, a warning for
plain HTTP, a keypair that never leaves its machine. State a protection that is deferred, and when it returns, in the
record that defers it, such as TLS before Landline is used off a tailnet. This does not permit an agent deferring a
protection without a person's approval, or calling a speed bump a boundary.

Records: 0040, 0060, 0200

### P8. Change only what the person chose

A command changes the thing it names and leaves the rest as it was. It checks that it can finish before it changes
anything, and a failure leaves the files as they were or says what state it left. It keeps a person's comments, a file
it did not write, and a file behind a symbolic link. A destructive action or a wider authorization waits for the
person's explicit act, and a narrower one takes effect at once. This does not permit a default that destroys, a rotation
that also revokes machines, or a write through a link into another repository.

Records: 0170, 0190, 0200, 0210, 0220

These principles are not points in a score. Privacy, correctness, ownership, and explicit commitments constrain
convenience. Within those constraints, reduce avoidable work and uncertainty, then prefer a smaller and more reversible
change.

## Known tensions

- **P3 and P2 on the default project.** A project found from the repository name saves a step, and it puts that name in
  the list for every agent and makes an accidental match between unrelated repositories likelier (0160). A match proves
  nothing, so a rule that treats one as consent is the decision of the skill that does so.
- **P5 and P6 on a refused request.** The switchboard knows why it refused a signed request, and the caller gets one
  text so that it learns nothing about which names exist (0200). The person needs the reason and the caller must not
  have it. Where the reason goes is a record's decision, not a principle's.
- **P8 and P3 on a confirmation.** `Rotate secret…` asks first (0170), and `Stop` and `Restart` do not, because the
  submenu is their guard (0140). A confirmation protects a person's sessions and adds a step to every use.
- **P4 and a compact screen.** The TUI shows a state as a shape and a legend and not as a word, so a screen reader hears
  the shape's name (0150). The word stays in `landline agents` and its JSON form. A setting to bring it back needs a
  stored format.

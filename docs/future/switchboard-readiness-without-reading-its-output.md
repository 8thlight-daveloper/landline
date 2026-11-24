# Switchboard readiness without reading its output

A direction and not a decision. Nothing here is built, and no record depends on it. Set down on 2026-11-24.

`landline server up` learns that the switchboard listens by finding a line that begins with
`landline-switchboard: listening on ` in `switchboard.out`. That line is written for people, so `up` depends on text
written for people, against coding principle C1.
[0220](../decisions/0220-the-switchboard-reads-its-settings-from-the-config-and-keeps-machines-json-in-the-data-directory.md)
anchors the match to the start of a line so that a label or a path cannot fake it, and leaves the dependency in place.

A channel built for programs would end it. `up` could pass the switchboard an inherited pipe, and the switchboard could
write one byte to it after it listens and close it. `up` waits for the byte, or for the pipe to close without one, which
means the switchboard stopped before it listened. The switchboard's exit status, `3` or `1`, then says why.

It needs its own record, because it changes how two programs talk to each other.

## Moving the switchboard to another host

[0200](../decisions/0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md) refuses
`join --code` in a home that has run a switchboard. A person who wanted to move the switchboard to another host without
downtime might have used such a join to hold a key for the old switchboard while setting up the new one. Nobody has
needed that yet. A record that supports moving a switchboard would decide how.

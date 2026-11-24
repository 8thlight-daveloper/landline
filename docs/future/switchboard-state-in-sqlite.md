# Switchboard state in SQLite

A direction and not a decision. Nothing here is built, and no record depends on it. Set down on 2026-11-24.

The switchboard keeps the signatures it has seen in memory, the joined machines in `machines.json`, and the message log
in a JSON lines file. It might move that state to SQLite: the seen signatures, the machines, and perhaps the log.

A database that commits each write would let the switchboard save every seen signature as it accepts the request,
without losing one in a crash. A restarted switchboard would then refuse every copy the old process accepted, and the
start period of
[0200](../decisions/0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md), which
refuses every signed request for one window after a start, could go.

It is deferred. SQLite adds a dependency to a project whose switchboard uses only the standard library, and it changes
the stored format of `machines.json`, and of the log if the log moves. Each of those needs its own record.

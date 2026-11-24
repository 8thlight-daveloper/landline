# Known edges

A list and not a decision. Nothing here is built. Set down on 2026-11-24.

The reviews of
[0200](../decisions/0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md) and
[0220](../decisions/0220-the-switchboard-reads-its-settings-from-the-config-and-keeps-machines-json-in-the-data-directory.md)
found cases that need a person to build something unusual inside Landline's own files, or a switchboard that sends
malformed HTTP. The person chose to record them here rather than block on them. A finding that loses data or the shared
secret, opens a security hole, breaks an accepted record or principle, or lets a wrong rebuild pass is not an edge, and
never belongs in this list.

- **A redirect with a `Location` header that does not parse.** Go's HTTP client reports the parse error before Landline
  sees the status, so the client prints that error in place of the redirect message of 0200. Nothing is sent to another
  place, and the request still fails.
- **A `machine.key` that is a symbolic link.** A `join --code` that fails after its request restores the old key's bytes
  and mode as a regular file, so the link to the person's key file is lost. The key still works.
- **A log name that differs from `machines.json` only in case, on a case-insensitive disk.** Covered once the
  switchboard makes `machines.json` before it opens the log and compares files by identity. Listed so that a rebuild
  keeps that order.

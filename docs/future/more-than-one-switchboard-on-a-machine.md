# More than one switchboard on a machine

A direction and not a decision. Nothing here is built, and no record depends on it. Set down on 2026-11-24.

[0220](../decisions/0220-the-switchboard-reads-its-settings-from-the-config-and-keeps-machines-json-in-the-data-directory.md)
lets one machine run one switchboard, under a lock in the data directory, because two switchboards would share
`machines.json` and erase each other's joins.

A person who wants two, such as a test switchboard beside the one in use, could get them if each switchboard kept its
state in files named for its port, or in a folder for each address: its own `machines.json`, lock, and log. A signed
request would then also need to name the switchboard it is for, so that a request one switchboard accepted cannot pass
the other.

It is deferred until someone has the need. It changes where `machines.json` lives and what a signature covers, and each
of those needs its own record.

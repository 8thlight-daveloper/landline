# Repowise

Record the selected Repowise version and installation method in the stack index. This setup uses the local open-source
engine with no model or embedding API spend. It is supporting development tooling, outside the shipped program and its
correctness tests.

## Operating constraints

Use local structural analysis, Git history, deterministic documentation, and keyword search. Do not enable hosted
services, model-written documentation, model-backed answers, or paid embedding providers. The coding agent's own usage
and local compute still have their usual costs.

Embeddings are numeric representations used for semantic similarity search; they are optional here. `mock` disables
meaningful semantic embeddings. Do not treat its results as semantic matches. Provider and embedder selection are
separate, and environment settings can override repository configuration. See
[configuration](https://docs.repowise.dev/installation/configuration).

## Initial setup

Install the open-source package in an isolated tool environment using the project's tool manager, then check
`repowise --version` and `repowise init --help`. Python requirements and supported installation methods are in the
[quickstart](https://docs.repowise.dev/getting-started/quickstart).

From the project root:

```sh
REPOWISE_EMBEDDER=mock repowise init --yes --no-prose --embedder mock --no-editor-setup --no-hook
repowise decision llm --off
```

The explicit `--no-prose` prevents model calls during initialization. Disabling editor setup and hooks keeps connection
changes separate from indexing. Check the installed version against the
[indexing reference](https://docs.repowise.dev/cli/indexing) before running these commands; preserve the no-spend
behavior if flags change.

Keep `embedder: mock` in `.repowise/config.yaml`, and set `REPOWISE_EMBEDDER=mock` for every Repowise process, including
the MCP server. Keep model-backed decision extraction disabled. Read accepted ADRs directly; do not depend on automated
extraction finding every decision. Do not configure API keys or model providers for this setup.

Keep the generated `.repowise/` index and local configuration out of version control. Add `/.repowise/` to `.gitignore`.
Maintain intended behavior and architecture rules in the project's existing documents and `architecture.yaml`.

## Connect the agent

Print the project's MCP connection snippet for the chosen host:

```sh
repowise agents print-config <agent-id> --scope project
```

Merge that connection into the project's existing agent configuration. Set the server's `REPOWISE_EMBEDDER` environment
entry to `mock`, and start it in this repository. Preserve the shared `AGENTS.md` and vendor import files. Avoid a
blanket agent setup that replaces instructions or changes global connections. The
[agent configuration reference](https://docs.repowise.dev/cli/agents) describes host-specific formats. Reload the host
and confirm its Repowise tools refer to the intended repository.

## Use during ordinary development

Read the assigned task, accepted decisions, and relevant contracts first. Use Repowise for focused repository questions
before broad source reads:

| Question                                | Tool                               | Check afterward                                    |
| --------------------------------------- | ---------------------------------- | -------------------------------------------------- |
| Where is the relevant subsystem?        | `get_overview`                     | Read the relevant maintained architecture document |
| What depends on these files or symbols? | `get_context` with batched targets | Confirm cited interfaces and index freshness       |
| What consumers could a change affect?   | `get_risk`                         | Inspect the relevant callers and contracts         |
| What risks does this diff introduce?    | `get_change_risk`                  | Review the actual diff and run the required checks |

The [context tool](https://docs.repowise.dev/mcp/get-context) supports bounded, batched retrieval. Request only the
evidence the task needs. Structural and keyword queries are sufficient for this workflow; do not enable a model-backed
answer or generation tool to compensate for an incomplete result.

Before relying on indexed facts, compare the served revision and worktree with the checkout under review. Refresh
structural context when needed:

```sh
REPOWISE_EMBEDDER=mock repowise update --index-only --working-tree --no-agents
```

This refresh skips documentation regeneration. Existing wiki summaries may remain stale; verify relevant facts against
current files. If indexing or MCP is unavailable, report the limitation and use targeted search and reads. Do not add a
paid fallback.

The index describes the implementation. Contracts and accepted decisions define what it must do. A graph or health score
does not authorize a protected change, prove correctness, or replace the project's dependency checks. Treat historical
co-change as evidence to investigate when choosing boundaries.

## Preserve independent regeneration

A builder regenerating a component must not learn its previous implementation or an independent reference build through
the index. That includes generated summaries, symbol bodies, skeletons, call flows, and implementation history.
Reviewers and coordinators may use the full index to investigate scope and prepare a brief.

Give the independent builder only the permitted contract, decisions, shared domain types, and dependency interfaces.
Inspect retrieved material before including it in that brief. If its tools cannot be restricted to those sources, do not
connect the builder to the full-repository index. Missing context is a specification question to resolve, not permission
to inspect a reference build.

## Verify the setup

Confirm the local index opens, MCP targets this checkout, external model and embedding calls are disabled, and a known
dependency query agrees with the source. Check that setup preserved agent instructions and existing hooks. Record the
version and any limitations, including language coverage and missing history, in the stack index. This is a local setup
check, not a claim that retrieval reduces cost or improves correctness on this project.

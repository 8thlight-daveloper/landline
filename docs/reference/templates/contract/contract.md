# <Component name> contract

Status: draft. Component: `<noun>/<role>`. Issue: <link>.

<One paragraph: what the component decides or produces, for whom, and what it does not do. Say that it is a pure function: it reads no file, keeps no state, and does not change its arguments. Link the reference docs and ADRs it draws on.>

## Interface

`<Call>(<argument>, <argument>) -> <result>`.

- `<argument>` is <type>. <What absent, empty, and nil mean.>
- `<argument>` is <type>.

The result is <what>. It depends only on <the arguments>. <Say whether there is an error or a problem list, and in what order problems come.>

## Rules

**<Term or rule>.** <One paragraph. Define the term the first time, then use only that word for it.>

**<Rule>.** <Use a table for case analysis.>

| Case | Result |
| -- | -- |
| <case> | <result> |

## Side effects and data access

- Side effects: none.
- Data access: <reads these fields> / <returns a copy that differs only in these fields> / <creates a value from these inputs>.

## Decisions to know

Each is a choice a reasonable reader could have made differently. `hard` means a person should read it before the contract binds a build.

| Decision | Reversal |
| -- | -- |
| <What was chosen, and the alternative> | cheap |
| <What was chosen, and the alternative> | hard |

## Open questions

None.

## Properties

These hold for every input the contract tests use.

- **Pure.** Every argument is the same after the call as before.
- **No memory.** The same call gives the same result on a shared instance, whatever calls came before, forward or in reverse order.
- **Deterministic.** Two calls give identical results.
- **Declared data access.** <Fields outside the declared set are unchanged.>

## Not covered

- <An input the contract gives no meaning to, and why, such as "the validator rejects it first".>

## Wrong implementations tried

Each row names a plausible wrong implementation and the cases that fail it. How the contract was checked, and with what scores, is recorded in the issue, not here.

| Wrong implementation | Caught by |
| -- | -- |
| <Ignores a condition> | `<case-name>` |
| <Mutates an argument> | The inputs-unchanged test |
| <Keeps a cache between calls> | The no-memory test |

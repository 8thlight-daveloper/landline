# Dump format

The protocol between `scripts/compare.py` and the two small programs it runs, one per build. Any language can implement
the program side.

## Input

On standard input: one JSON array of strings. Each string is one input to the component, base64-encoded so binary and
odd text survive.

## Output

On standard output: one JSON array of strings, in the same order as the input. Each string is the result of one input,
in this form:

```text
<the returned value, printed on one line>
<code>|<severity>|<location>|<details>
<code>|<severity>|<location>|<details>
```

- **Line 1** is the component's returned value, printed structurally: field names and values, maps with sorted keys,
  lists in order, absent and empty told apart (`nil[]` versus `[]`), references followed rather than printed as
  addresses.
- **Each later line** is one reported problem: its stable code, its severity, where it points (such as a path into the
  input), and any further detail. `compare.py` groups differences by code and location.
- **A crash** is caught and printed as the value `PANIC <message>`, so one bad input does not stop the run.

The program must be deterministic. Two runs on the same corpus give identical output.

## Writing an adapter

1. Read the array, decode each item.
1. Call the component through its public interface, as the contract does.
1. Print the value with a structural printer, not the language's default debug print.
1. Print each problem on its own line in the format above.
1. Catch crashes per input.

[adapters/go/dump.go.tmpl](../scripts/adapters/go/dump.go.tmpl) is a worked example. Fill in
`callComponent(data []byte)` to call the component and return its value and problems.

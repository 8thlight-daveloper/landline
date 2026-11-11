---
name: Build
about: Build one component from its approved contract
labels: build
---

## Component

Name and path, for example `section/selector` and `<components>/section/selector/`.

## Contract

Link to the approved contract. Do not start until it is merged.

## Done when

- [ ] Every contract test failed against a stub first, then passes
- [ ] Architecture tests pass
- [ ] Surviving mutants are reported in a comment as contract gaps
- [ ] Only the component's folder changed
- [ ] Any conflict with the contract is reported in a comment, and the contract is unedited

## Out of scope

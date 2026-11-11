# Mutation recipes for a Go project, the Go implementation of the recipes in
# process/task-recipes.md. Copy this folder to tools/mutation/ and add
# `import "tools/mutation/mutation.just"` to the project's justfile.

components_dir := "internal"
contracts_dir := "contracts"
accepted_dir := "tools/mutation/accepted"
gomu_version := "v0.3.0"

# Mutation-test one component against its contract, for example `just mutate plan/encoder`. It runs gomu over an overlay, so it never changes a source file and can run beside `just check`. One mutant at a time by default, with Go limited to two parallel jobs. Survivors are gaps in the contract, except those a person accepted in the accepted folder. Pass `--workers 2` to go faster.
mutate component *flags:
    mkdir -p .direnv/bin
    stamp="{{ gomu_version }} $(go env GOVERSION)"; if [ ! -x .direnv/bin/gomu ] || [ "$(cat .direnv/bin/gomu.stamp 2>/dev/null)" != "$stamp" ]; then GOBIN="$(pwd)/.direnv/bin" go install github.com/sivchari/gomu/cmd/gomu@{{ gomu_version }} && echo "$stamp" > .direnv/bin/gomu.stamp; fi
    go run ./tools/mutation/gomurun --components {{ components_dir }} --contracts {{ contracts_dir }} --accepted {{ accepted_dir }} --gomu-version {{ gomu_version }} {{ flags }} {{ component }}

# Print a JSON array of the components to mutate, from the changes since a Git revision. An empty or unknown revision plans every component.
mutate-plan base="":
    COMPONENTS_DIR={{ components_dir }} CONTRACTS_DIR={{ contracts_dir }} bash tools/mutation/plan.sh "{{ base }}"

# Summarize one mutation run from its log, and write result.json.
mutate-summary component log seconds:
    bash tools/mutation/summarize.sh "{{ component }}" "{{ log }}" "{{ seconds }}"

# Combine every result.json under a folder into one Markdown report.
mutate-report results:
    bash tools/mutation/report.sh "{{ results }}"

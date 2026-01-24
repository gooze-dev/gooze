![logo](./logo.png)

A Golang mutation testing tool inspired by TMNT "ooze" mutagen.

Gooze helps you measure test suite quality by introducing small, controlled mutations
into your Go source and running tests to see which changes are caught. It supports
Go path patterns (like `./...`) for fast targeting and can parallelize mutation runs
to speed up larger projects.

## Quick Start

### Install

Install the latest `gooze` binary to your Go bin directory.

```bash
go install github.com/mouse-blink/gooze@latest
```


### List files and mutation counts

Preview which files will be mutated and how many mutations apply.

```bash
gooze list ./...
```

### Run mutation testing

Execute mutation testing across the target paths.

```bash
gooze run ./...
```

With parallel workers:

```bash
gooze run -p 4 ./...
```

> Tips:
> - Use `gooze list` to preview the files and mutation counts before running tests.
> - Use `--parallel` to reduce total runtime on multi-core machines.

### UI modes

Gooze automatically selects the UI based on whether output is a TTY:

- **Interactive TUI**: Used when running in a terminal.
- **Simple/CI UI**: Used when output is redirected or in CI.

To skip the interactive UI, pipe output (e.g., `gooze run ./... | cat`).


## Complete Go Mutation Testing Categories

- [x] Boolean Literal
- [ ] Numbers
- [x] Unary / Negation
- [x] Arithmetic
- [x] Comparison / Relational
- [x] Logical Operators
- [ ] Core Logic
- [ ] Statement
- [ ] Statement Deletion
- [ ] Return Value
- [ ] Branch
- [ ] Conditional
- [ ] Loop
- [ ] Control Flow & Loops
- [ ] Expression
- [ ] Complex Expression
- [ ] Slice
- [ ] Map
- [ ] Pointer & Memory
- [ ] Interface / Type Assertion
- [ ] Function Signature / Parameter
- [ ] Type System & Interfaces
- [ ] Global State & Initialization
- [ ] Go-Specific Error Handling
- [ ] Concurrency & Channels

## Roadmap

### Smart Test Execution
- [ ] Run only matching `*_test.go` files for each mutated source file
- [ ] Reduces test execution time by running relevant tests only

### Git Integration
- [ ] Mutate only files changed in git diff
- [ ] Focus mutation testing on modified code

### Performance & Scalability
- [x] `--parallel` flag for concurrent mutation testing
- [x] Sharding support for distributed execution across multiple machines
- [x] Compatible with parallel execution within shards
- [ ] Automatic report merging from multiple shards

### Reporting
- [ ] OCI artifact-based reports stored as container images
- [ ] Per-file mutation reports for granular analysis
- [ ] Index file with summary and aggregated metrics
- [ ] Incremental testing: merge branch results with master OCI artifacts for unchanged files

### CI/CD Integration
- [ ] GitHub Actions workflow templates
- [ ] GitLab CI pipeline configuration

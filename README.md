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

### Reports

By default, Gooze writes mutation reports to `.gooze-reports` (override with `-o/--output`).

- One YAML file per report: `<hash>.yaml`
- An index file: `index.yaml`

View the last run:

```bash
gooze view
```

Or point `view` at an explicit directory:

```bash
gooze view -o .gooze-reports
```

#### Sharded runs and merging

When sharding is enabled (`-s/--shard INDEX/TOTAL`), reports are written to shard subdirectories:

- `<output>/shard_0/`
- `<output>/shard_1/`
- ...

Example distributed run (3 shards) and merge:

```bash
gooze run -o .gooze-reports -s 0/3 ./...
gooze run -o .gooze-reports -s 1/3 ./...
gooze run -o .gooze-reports -s 2/3 ./...

gooze merge -o .gooze-reports
gooze view -o .gooze-reports
```

With parallel workers:

```bash
gooze run -p 4 ./...
```

Exclude files by regex (repeatable):

```bash
gooze run -x '^vendor/' -x '^mock_' ./...
```

> Tips:
> - Use `gooze list` to preview the files and mutation counts before running tests.
> - Use `--parallel` to reduce total runtime on multi-core machines.
> - Use `-x`/`--exclude` to skip files by regex (path or base name).

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
- [x] Run only matching `*_test.go` files for each mutated source file
- [x] Reduces test execution time by running relevant tests only

### Performance & Scalability
- [x] `--parallel` flag for concurrent mutation testing
- [x] Sharding support for distributed execution across multiple machines
- [x] Compatible with parallel execution within shards
- [x] Automatic report merging from multiple shards (`gooze merge`)

### Reporting
- [ ] OCI artifact-based reports stored as container images
- [x] Per-file mutation reports for granular analysis
- [x] Index file with summary (`index.yaml`)
- [ ] Incremental testing: merge branch results with master OCI artifacts for unchanged files

### CI/CD Integration
- [ ] GitHub Actions workflow templates
- [ ] GitLab CI pipeline configuration

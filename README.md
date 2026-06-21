![logo](./logo.png)

A Golang mutation testing tool inspired by TMNT "ooze" mutagen.

Gooze helps you measure test suite quality by introducing small, controlled mutations
into your Go source and running tests to see which changes are caught. It supports
Go path patterns (like `./...`) for fast targeting and can parallelize mutation runs
to speed up larger projects.

**Choosing a tool?** See [COMPARISON.md](./COMPARISON.md).

## Quick Start

### Install

Install the latest `gooze` binary to your Go bin directory.

```bash
go install gooze.dev/pkg/gooze@latest
```

`gooze.dev/pkg/gooze` is a vanity import path. For this to work, `https://gooze.dev/pkg/gooze?go-get=1` must serve a `go-import` meta tag pointing at this repo (template: `docs/pkg/gooze/index.html`).

### Commands

```
gooze run [paths...]            Run mutation testing
      run --estimate            Preview files + applicable mutation counts (no tests run)
      run --coverage-profile    Skip mutations on uncovered lines using a Go coverage profile
gooze report view               View previously generated reports
       report merge             Merge sharded reports into one directory
       report push <reference>  Push reports to an OCI registry as an artifact
       report pull <reference>  Pull reports from an OCI registry
gooze config init               Generate a default gooze.yaml
gooze version                   Show version information
```

Run `gooze <command> --help` (e.g. `gooze report --help`) for full flags.

### Preview files and mutation counts

Preview which files will be mutated and how many mutations apply, without running tests.

```bash
gooze run --estimate ./...
```

### Run mutation testing

Execute mutation testing across the target paths. With no paths, `run` defaults
to `./...` (the current module, recursively).

```bash
gooze run          # same as: gooze run ./...
gooze run ./pkg/...
```

### Skip mutations on uncovered lines

Pass a Go coverage profile and gooze will report any mutation on a line that the
profile shows as uncovered as `not_covered` — immediately, without running its
tests. This speeds up runs (uncovered mutations can never be killed) and makes
test gaps explicit. `not_covered` counts as a survivor in the mutation score and
is tallied separately (`not_covered_mutations` in `_index.yaml`).

```bash
go test -coverprofile=coverage.out ./...
gooze run --coverage-profile coverage.out ./...
```

### Config File Support (`.gooze.yml`)

Gooze supports a configuration file (`.gooze.yml`) for persistent settings, reducing the need to specify options repeatedly on the command line. Place the file in the root of your project or specify its location with the `--config` flag.

**Example `.gooze.yml`:**

```yaml
output: .gooze-reports
parallel: 4
exclude:
  - '^vendor/'
  - '^mock_'
shard: 0/3
```

**Supported Options:**
- `output`: Directory for mutation reports (default: `.gooze-reports`).
- `parallel`: Number of parallel workers for mutation testing.
- `exclude`: List of regex patterns to exclude files.
- `shard`: Shard index and total (e.g., `0/3` for shard 0 of 3).
- `no_cache`: Boolean to disable caching (default: `false`).

#### Environment variables

Gooze also reads configuration from environment variables. Keys are mapped from config keys by:

- Prefixing with `GOOZE_`
- Uppercasing
- Replacing `.` and `-` with `_`

Flag precedence is generally: CLI flags → env vars → config file → defaults. For logging specifically, `--verbose` and `--log-output` override env/config.

| Config key | Env var | Type | Default | Notes |
|---|---|---:|---|---|
| `output` | `GOOZE_OUTPUT` | string | `.gooze-reports` | Reports output directory |
| `no-cache` | `GOOZE_NO_CACHE` | bool | `false` | When `true`, disables incremental cache |
| `paths.exclude` | `GOOZE_PATHS_EXCLUDE` | string list | `[]` | Comma-separated (e.g. `^vendor/,^mock_`) |
| `run.parallel` | `GOOZE_RUN_PARALLEL` | int | `1` | Worker count for `run` |
| `run.mutation_timeout` | `GOOZE_RUN_MUTATION_TIMEOUT` | int | `120` | Per-mutation timeout (seconds) (also `--mutation-timeout`) |
| `run.coverage_profile` | `GOOZE_RUN_COVERAGE_PROFILE` | string | `""` | Go coverage profile path; mutations on uncovered lines become `not_covered` (also `--coverage-profile`) |
| `log.filename` | `GOOZE_LOG_FILENAME` | string | `.gooze.log` | Log file path (also settable via `--log-output`) |
| `log.verbose` | `GOOZE_LOG_VERBOSE` | bool | `false` | When `true`, forces debug logging (also `--verbose`) |
| `log.level` | `GOOZE_LOG_LEVEL` | string/int | `info` | `debug`, `info`, `warn`, `error` (or numeric slog level) |
| `log.max_size` | `GOOZE_LOG_MAX_SIZE` | int | `10` | MiB before rotation |
| `log.max_backups` | `GOOZE_LOG_MAX_BACKUPS` | int | `3` | Number of rotated files to keep |
| `log.max_age` | `GOOZE_LOG_MAX_AGE` | int | `28` | Days to keep old logs |
| `log.compress` | `GOOZE_LOG_COMPRESS` | bool | `true` | Gzip rotated logs |

**Usage:**
- Gooze automatically loads `.gooze.yml` if present in the current directory.
- Override specific options via command-line flags (e.g., `gooze run --output custom-dir ./...`).

> **Tip:** Use `.gooze.yml` to standardize settings across your team and CI pipelines.

### Reports

By default, Gooze writes mutation reports to `.gooze-reports` (override with `-o/--output`).

- One YAML file per report: `<hash>.yaml`
- An index file: `_index.yaml`

View the last run:

```bash
gooze report view
```

Or point it at an explicit directory:

```bash
gooze report view -o .gooze-reports
```

### Incremental runs (`--no-cache`)

Gooze supports incremental mutation testing by caching results and skipping unchanged files (use `--no-cache` to ignore the cache and re-test everything).

**How it works:**

1. After running tests, Gooze stores mutation results in the reports directory (default `.gooze-reports/`, configurable with `-o`) with source file hashes
2. On subsequent runs, Gooze checks each source file:
   - If source or test file content changed → re-run mutations
   - If mutator versions changed → re-run mutations
   - Otherwise → skip (use cached results)

**Example**

First run populates the cache:

```bash
gooze run ./...
```

Make a small change to a single file:

```bash
echo "// comment" >> main.go
```

Second run re-tests only affected sources and reuses cached results for everything else:

```bash
gooze run ./...
```

To ignore the cache and force re-testing everything:

```bash
gooze run --no-cache ./...
```

**Cache invalidation triggers:**
- Source file content hash changed
- Test file content hash changed
- Mutator version changed (e.g., after upgrading Gooze)
- Source file deleted

### Storing reports in an OCI registry

Store and retrieve mutation reports as OCI artifacts in any registry — built in
via `gooze report push` / `pull` (powered by [ORAS](https://oras.land/)), so no
external `oras` or `tar` steps are needed.

`push`/`pull` operate on the reports directory (`-o/--output`, default
`.gooze-reports`). Flags: `--plain-http` (non-TLS registry), `--insecure` (skip
TLS verification).

**Authentication.** For registries that require login, set
`GOOZE_REGISTRY_USERNAME` and `GOOZE_REGISTRY_PASSWORD`. The password may be a
token/PAT — e.g. for GHCR use your GitHub username with a PAT (or `GITHUB_TOKEN`
in CI):

```bash
export GOOZE_REGISTRY_USERNAME="$USER"
export GOOZE_REGISTRY_PASSWORD="$GITHUB_TOKEN"
gooze report push ghcr.io/your-org/your-repo/gooze-reports:main
```

If unset, gooze falls back to the Docker credential store (whatever
`docker login` saved).

**Push reports:**

```bash
gooze run -o .gooze-reports ./...
gooze report push ghcr.io/your-org/your-repo/gooze-reports:main
```

**Pull and view reports:**

```bash
gooze report pull ghcr.io/your-org/your-repo/gooze-reports:main
gooze report view
```

**Incremental testing in CI** — restore the baseline, run, publish:

```bash
gooze report pull ghcr.io/your-org/your-repo/gooze-reports:main   # restore baseline
gooze run ./...                                                    # only changed sources re-tested
gooze report push ghcr.io/your-org/your-repo/gooze-reports:main   # publish updated reports
```

**Benefits:**
- Reuse cached results across CI runs
- Speed up branch testing by reusing main branch results
- Version and track mutation test results alongside code
- Share baseline reports across team members

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

gooze report merge -o .gooze-reports
gooze report view -o .gooze-reports
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
> - Use `gooze run --estimate` to preview the files and mutation counts before running tests.
> - Use `--parallel` to reduce total runtime on multi-core machines.
> - Use `-x`/`--exclude` to skip files by regex (path or base name).

### UI modes

Gooze automatically selects the UI based on whether output is a TTY:

- **Interactive TUI**: Used when running in a terminal.
- **Simple/CI UI**: Used when output is redirected or in CI.

To skip the interactive UI, pipe output (e.g., `gooze run ./... | cat`).

### Annotation skipping (`//gooze:ignore`)

Skip generating mutations by placing a single annotation: `//gooze:ignore`.
You can optionally provide a comma-separated list of mutagen names, e.g. `//gooze:ignore arithmetic,comparison`.

Mutagen names match the labels shown in output, e.g. `arithmetic`, `comparison`, `numbers`, `boolean`, `logical`, `unary`, `branch`, `statement`, `loop`.

Scope is determined by *where* the annotation appears:

- **File**: if the annotation appears before the `package` declaration (typically the first line), it applies to the whole file.
- **Function / method**: if the annotation is immediately above a `func` declaration, it applies to that entire function/method.
- **Line**: if the annotation appears on its own line directly above a statement, or as a trailing comment on the same line, it applies only to that line.

Examples:

```go
//gooze:ignore arithmetic,comparison
package main

//gooze:ignore
func main() {
   x := 1 + 2 //gooze:ignore numbers
   if x > 0 { //gooze:ignore comparison
      println(x)
   }

   //gooze:ignore
   y := x + 1
   _ = y
}
```


## Complete Go Mutation Testing Categories

- [x] Boolean Literal
- [x] Numbers
- [x] Unary / Negation
- [x] Arithmetic
- [x] Comparison / Relational
- [x] Logical Operators
- [x] Branch (if/else removal, condition inversion, switch case removal)
- [x] Statement (statement deletion: assignments, expressions, defer, go, send)
- [x] Loop (boundary conditions, loop body removal, break/continue removal)
- [ ] Core Logic
- [ ] Return Value
- [ ] Conditional
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

### Core Features
- [x] **Annotation Skipping**: Support `//gooze:ignore` to skip file/function/line, optionally per mutagen (Medium)
- [ ] **Custom Exec Hook**: Support custom test runner commands similar to `go-mutesting --exec` (High)
- [ ] **Function Selection**: Allow mutating specific functions/methods via regex (High)
- [x] **Timeouts**: Per-mutation execution budgets to prevent infinite loops (Medium)
- [x] **Config File**: Support `.gooze.yml` for persistent configuration (Medium)

### Smart Test Execution
- [x] Run only matching `*_test.go` files for each mutated source file
- [x] Reduces test execution time by running relevant tests only

### Performance & Scalability
- [x] `--parallel` flag for concurrent mutation testing
- [x] Sharding support for distributed execution across multiple machines
- [x] Compatible with parallel execution within shards
- [x] Automatic report merging from multiple shards (`gooze report merge`)

### Reporting
- [x] Incremental testing: cache and reuse results for unchanged files
- [x] Per-file mutation reports for granular analysis
- [x] Index file with summary (`_index.yaml`)
- [ ] OCI artifact integration with automated push/pull workflows

### CI/CD Integration
- [ ] GitHub Actions workflow templates
- [ ] GitLab CI pipeline configuration

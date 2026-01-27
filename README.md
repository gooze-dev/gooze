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
- An index file: `_index.yaml`

View the last run:

```bash
gooze view
```

Or point `view` at an explicit directory:

```bash
gooze view -o .gooze-reports
```

### Incremental runs

Gooze supports incremental mutation testing by caching results and skipping unchanged files.

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

**Cache invalidation triggers:**
- Source file content hash changed
- Test file content hash changed
- Mutator version changed (e.g., after upgrading Gooze)
- Source file deleted

### Using ORAS for report storage

Store and retrieve mutation reports as OCI artifacts using [ORAS](https://oras.land/).

**Push reports to registry:**

Run mutation testing and write reports to `.gooze-reports`:

```bash
gooze run -o .gooze-reports ./...
```

Package reports into a single archive (this avoids nested paths on pull):

```bash
tar -C .gooze-reports -czf gooze-reports.tgz .
```

Push the archive as an OCI artifact:

```bash
oras push ghcr.io/your-org/your-repo/gooze-reports:main \
   gooze-reports.tgz:application/gzip

rm -f gooze-reports.tgz
```

**Pull reports from registry:**

Pull the artifact to a staging directory:

```bash
rm -rf /tmp/gooze-reports-oci && mkdir -p /tmp/gooze-reports-oci
oras pull ghcr.io/your-org/your-repo/gooze-reports:main -o /tmp/gooze-reports-oci
```

Restore into the reports directory Gooze reads from:

```bash
rm -rf .gooze-reports && mkdir -p .gooze-reports
tar -C .gooze-reports -xzf /tmp/gooze-reports-oci/gooze-reports.tgz
```

View the pulled reports:

```bash
gooze view -o .gooze-reports
```

**Incremental testing with OCI artifacts:**

In CI, restore baseline reports first:

```bash
rm -rf /tmp/gooze-reports-oci && mkdir -p /tmp/gooze-reports-oci
oras pull ghcr.io/your-org/your-repo/gooze-reports:main -o /tmp/gooze-reports-oci
rm -rf .gooze-reports && mkdir -p .gooze-reports
tar -C .gooze-reports -xzf /tmp/gooze-reports-oci/gooze-reports.tgz
```

Then run mutation testing; only changed sources will be re-tested:

```bash
gooze run -o .gooze-reports ./...
```

Finally, package and push the updated reports:

```bash
tar -C .gooze-reports -czf gooze-reports.tgz .
oras push ghcr.io/your-org/your-repo/gooze-reports:feature-branch \
   gooze-reports.tgz:application/gzip
rm -f gooze-reports.tgz
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
- [x] Numbers
- [x] Unary / Negation
- [x] Arithmetic
- [x] Comparison / Relational
- [x] Logical Operators
- [x] Branch (if/else removal, condition inversion, switch case removal)
- [x] Statement (statement deletion: assignments, expressions, defer, go, send)
- [ ] Core Logic
- [ ] Return Value
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
- [x] Incremental testing: cache and reuse results for unchanged files
- [x] Per-file mutation reports for granular analysis
- [x] Index file with summary (`_index.yaml`)
- [ ] OCI artifact integration with automated push/pull workflows

### CI/CD Integration
- [ ] GitHub Actions workflow templates
- [ ] GitLab CI pipeline configuration

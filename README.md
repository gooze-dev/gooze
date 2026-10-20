![logo](./logo.png)

A Golang mutation testing tool inspired by TMNT "ooze" mutagen.

Gooze helps you measure test suite quality by introducing small, controlled mutations
into your Go source and running tests to see which changes are caught. It supports
Go path patterns (like `./...`) for fast targeting and can parallelize mutation runs
to speed up larger projects.

**Choosing a tool?** See [COMPARISON.md](./COMPARISON.md).

## Install

```bash
go install gooze.dev/pkg/gooze@latest
```

`gooze.dev/pkg/gooze` is a vanity import path: `https://gooze.dev/pkg/gooze?go-get=1`
serves a `go-import` meta tag pointing at this repository.

## Quick start

```bash
gooze run --estimate ./...   # preview files and mutation counts, no tests run
gooze run ./...              # run mutation testing
gooze report view            # view the last run's reports
```

## Documentation

The full documentation lives at <https://gooze.dev/docs>:

- [Quick start](https://gooze.dev/docs/quick-start)
- CLI reference: [`gooze run`](https://gooze.dev/docs/cli/run), [`gooze report`](https://gooze.dev/docs/cli/report)
- [Configuration](https://gooze.dev/docs/config) (`gooze.yaml`, flags, environment variables)
- [Bypass mutation](https://gooze.dev/docs/features/bypass-mutation) (`//gooze:ignore`)
- [Incremental execution](https://gooze.dev/docs/features/incremental-execution) and [OCI reports](https://gooze.dev/docs/cli/report/#push)
- CI guides: [GitHub Actions](https://gooze.dev/docs/ci), [GitLab CI](https://gooze.dev/docs/gitlab)
- [Troubleshooting](https://gooze.dev/docs/troubleshooting)

### Not yet on the docs site

Log rotation settings (config key / env var, defaults shown):

| Config key | Env var | Default | Notes |
|---|---|---|---|
| `log.max_size` | `GOOZE_LOG_MAX_SIZE` | `10` | MiB before rotation |
| `log.max_backups` | `GOOZE_LOG_MAX_BACKUPS` | `3` | Number of rotated files to keep |
| `log.max_age` | `GOOZE_LOG_MAX_AGE` | `28` | Days to keep old logs |
| `log.compress` | `GOOZE_LOG_COMPRESS` | `true` | Gzip rotated logs |

`gooze run` rejects out-of-range settings, whether they come from flags,
environment variables or `gooze.yaml`: `version` must be `1`, and `run.parallel`
and `run.mutation_timeout` must be at least `1`. `log.level` takes `debug`, `info`
(the default), `warn` or `error`, or a numeric slog level.

Cached results for a file are invalidated when its source content hash changes,
its test file content hash changes, a mutator version changes (e.g. after upgrading
Gooze), or the source file is deleted.

Mutation categories not implemented yet: core logic, return value, conditional,
complex expression, slice, map, pointer & memory, interface / type assertion,
function signature / parameter, type system & interfaces, global state &
initialization, Go-specific error handling, concurrency & channels.

## Go packages

Tools that work with gooze (editors, language servers, CI) can import its file
formats instead of re-implementing them. These packages depend only on the
standard library and `gopkg.in/yaml.v3`:

- [`gooze.dev/pkg/gooze/pkg/config`](https://pkg.go.dev/gooze.dev/pkg/gooze/pkg/config): the `gooze.yaml` schema (keys, kinds, defaults, bounds, docs) and the config file name.
- [`gooze.dev/pkg/gooze/pkg/report`](https://pkg.go.dev/gooze.dev/pkg/gooze/pkg/report): report and `_index.yaml` files, test statuses, and the mutation score.
- [`gooze.dev/pkg/gooze/pkg/directive`](https://pkg.go.dev/gooze.dev/pkg/gooze/pkg/directive): the `//gooze:ignore` parser, with byte offsets.
- [`gooze.dev/pkg/gooze/pkg/mutagen`](https://pkg.go.dev/gooze.dev/pkg/gooze/pkg/mutagen): the mutagen registry (names, versions, docs).

## Development

```bash
make install-tools   # cobra-cli, pre-commit, golangci-lint, mockery, semgrep
make build           # build ./.bin/gooze
make test            # run the tests with coverage
make lint            # golangci-lint
make semgrep         # semgrep with the rules in .semgrep/
make mocks           # regenerate mocks from .mockery.yaml
pre-commit install   # or: make install-precommit
```

The repository runs mutation testing on itself on every pull request
(`.github/workflows/mutation.yml`), using the baseline from `main` for incremental runs.

## License

[MIT](./LICENSE)

# Gooze Comparison

| Tool | Gooze | go-mutesting (Original) |
|---|---|---|
| **Repo** | `gooze.dev/pkg/gooze` | `github.com/zimmski/go-mutesting` |
| **Philosophy** | **"Batteries Included"**: Modern, fast, incremental, distributed-ready. | **"Functionality First"**: Minimal, extensible framework relying on external scripting. |
| **Integrations** | Built-in TUI, persistent storage, native sharding. | Relies on custom `--exec` scripts/hooks. |
| **Status** | Active, focused on DX and performance. | Stable, mature, minimal feature set. |

| Decision Matrix | Recommended | Why? |
|---|---|---|
| **Developer Experience** | **Gooze** | Incremental caching enables rapid "edit-test" loops locally; TUI provides clear visual feedback. |
| **CI/CD Integration** | **Gooze** | Native sharding (`--shard`) and report merging allows distributed execution across agents. |
| **Performance** | **Gooze** | Caches results by file hash; only re-runs affected tests on subsequent runs. |
| **Feature Set** | **Gooze** | 9 mutation categories out-of-the-box vs 3 in original go-mutesting. |
| **Ease of Use** | **Gooze** | Simple `list`, `run`, `view` workflow without complex shell arguments. |
| **Custom scripting** | go-mutesting | Best if you need absolute control over the execution process via external binaries/scripts. |
| **Minimalism** | go-mutesting | Best if you want a bare-bones framework to build your own tools on top of. |

| Other Go mutation testing options | Repo / Approach | Strengths | Tradeoffs |
|---|---|---|---|
| go-mutesting (fork/variant) | `github.com/avito-tech/go-mutesting` (fork of `zimmski/go-mutesting`) | Useful if your team already standardized on that fork | Feature set varies by fork; still lacks Gooze-style caching/sharding/report UX |
| go-mutesting (other forks) | Any fork of `zimmski/go-mutesting` | Familiar baseline; easy to patch for internal needs | Fork maintenance burden; still largely “run everything” and script-driven |
| DIY mutator + scripts | Custom `go/ast` mutator + `go test` wrapper | Maximum flexibility and tight integration with bespoke build systems | High effort to build/maintain; you end up re-implementing reporting, caching, and orchestration |

| Installation & CLI | Gooze | go-mutesting |
|---|---|---|
| **Install** | `go install gooze.dev/pkg/gooze@latest` | `go get -t -v github.com/zimmski/go-mutesting/...` |
| **Binary** | `gooze` | `go-mutesting` |
| **Discovery** | `gooze list ./...` (cached) | `go-mutesting --list-files ./...` |
| **Execution** | `gooze run ./...` (cached) | `go-mutesting ./...` (always runs all) |
| **Reporting** | `gooze view` (reads persistent store) | Stdout / Diff output only |
| **Sharding** | `gooze run -s 1/3 ./...` + `gooze merge` | Not supported natively |

| Feature Details | Gooze | go-mutesting |
|---|---|---|
| **Incremental Cache** | ✅ Built-in (file hash + mutation version) | ❌ Manual MD5 blacklist file only |
| **Parallelism** | ✅ `--parallel 4` / `-p 4` flag (threads) | ⚠️ Via custom `--exec` scripts only |
| **File Exclusion** | ✅ Regex via `--exclude` / `-x` | ⚠️ Via build tags or `grep` |
| **Output format** | ✅ Persistent YAML + Index in `.gooze-reports` | ⚠️ Stdout / Temporary files |
| **Mutation Types** | ✅ 9 Types (Arithmetic, Branch, Logical, etc.) | ⚠️ 3 Types (Branch, Expression, Statement) |
| **False Positives** | ✅ Auto-suppressed via cache (don't re-run) | ⚠️ Manual blacklist file maintenance required |
| **Annotation Skipping** | ❌ Not supported yet (See Roadmap) | ❌ Not supported |

| Roadmap / Gaps | Tool | Priority | Item | Notes |
|---|---|---|---|---|
| Feature Request | Gooze | Medium | **Annotation Skipping** | Support `//gooze:ignore` comments |
| Gap vs go-mutesting | Gooze | High | Custom exec hook | Similar to `go-mutesting --exec` flexibility for custom runners |
| Gap vs go-mutesting | Gooze | High | Function selection | `go-mutesting --match` equivalent |
| Gap vs go-mutesting | Gooze | Medium | Timeouts | per-mutation budgets |
| Gap vs Gooze | go-mutesting | High | **Incremental caching** | The #1 performance feature for large codebases |
| Gap vs Gooze | go-mutesting | High | **Sharding** | Critical for CI pipelines |
| Gap vs Gooze | go-mutesting | Medium | **Persistent reports** | Ability to review results later / offline |

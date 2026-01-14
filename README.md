![logo](./logo.png)

A Golang mutation testing tool inspired by TMNT "ooze" mutagen.



## Complete Go Mutation Testing Categories

- [x] Boolean Literal
- [ ] Numbers
- [ ] Unary / Negation
- [x] Arithmetic
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
- [ ] `--parallel` flag for concurrent mutation testing
- [ ] Sharding support for distributed execution across multiple machines
- [ ] Compatible with parallel execution within shards
- [ ] Automatic report merging from multiple shards

### Reporting
- [ ] OCI artifact-based reports stored as container images
- [ ] Per-file mutation reports for granular analysis
- [ ] Index file with summary and aggregated metrics
- [ ] Incremental testing: merge branch results with master OCI artifacts for unchanged files

### CI/CD Integration
- [ ] GitHub Actions workflow templates
- [ ] GitLab CI pipeline configuration

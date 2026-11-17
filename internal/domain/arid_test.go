package domain_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domain "gooze.dev/pkg/gooze/internal/domain"
	m "gooze.dev/pkg/gooze/internal/model"
	mgen "gooze.dev/pkg/gooze/pkg/mutagen"
)

// aridLines returns the lines of src that still have mutants under arid, and
// the lines whose mutants it leaves out as arid, each once and in order.
func aridLines(t *testing.T, src string, arid domain.AridOptions, types ...mgen.Type) (kept, left []int) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "p.go")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	mg, ok := newTestMutagen().(domain.MutagenWithOptions)
	require.True(t, ok)

	lines := func(ignored bool) []int {
		var got []int

		err := mg.StreamMutationsWith(context.Background(), makeSourceV2(t, path), domain.MutationOptions{Arid: arid, Ignored: ignored}, func(mutation m.Mutation) error {
			if ignored {
				require.NotNil(t, mutation.Ignored)
				require.Equal(t, domain.AridReason, mutation.Ignored.Reason)
			}

			got = append(got, mutation.Line)

			return nil
		}, types...)
		require.NoError(t, err)

		slices.Sort(got)

		return slices.Compact(got)
	}

	return lines(false), lines(true)
}

var aridOn = domain.AridOptions{Enabled: true}

const aridLogging = `package p

import (
	"log"
	"log/slog"
)

type svc struct{ logger *slog.Logger }

func (s *svc) run(n int) int {
	log.Printf("got %d", n+1)
	slog.Info("x", "n", n*2)
	s.logger.Debug("y", "n", n-1)
	s.logger.With("a", n+3).Info("z")
	if n > 10 {
		log.Println("big", n+4)
	}
	if n > 20 {
		log.Println("huge", n+8)
		return n + 5
	}
	log.Fatalf("bad %d", n+6)
	return n + 7
}
`

func TestArid_LoggingCalls(t *testing.T) {
	t.Parallel()

	kept, left := aridLines(t, aridLogging, aridOn, mgen.Arithmetic, mgen.Comparison)
	assert.Equal(t, []int{18, 20, 22, 23}, kept, "a condition with more than logging under it, a fatal log and the returns stay")
	assert.Equal(t, []int{11, 12, 13, 14, 15, 16, 19}, left)
}

func TestArid_OffByDefault(t *testing.T) {
	t.Parallel()

	kept, left := aridLines(t, aridLogging, domain.AridOptions{}, mgen.Arithmetic, mgen.Comparison)
	assert.Equal(t, []int{11, 12, 13, 14, 15, 16, 18, 19, 20, 22, 23}, kept)
	assert.Empty(t, left)

	kept, left = aridLines(t, aridLogging, domain.AridOptions{Rules: []string{"logging"}, Calls: []string{"log.*"}}, mgen.Arithmetic)
	assert.Len(t, kept, 9, "rules and calls alone don't turn it on")
	assert.Empty(t, left)
}

const aridTimeAndFlags = `package p

import (
	"context"
	"flag"
	"net"
	"time"
)

var retries = flag.Int("retries", 3+1, "how often")

func wait(ctx context.Context, conn net.Conn, n int) int {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	time.Sleep(time.Duration(n+1) * time.Millisecond)
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	time.AfterFunc(2*time.Second, func() { n = n + 9 })
	select {
	case <-time.After(2 * time.Second):
	case <-ctx.Done():
	}
	return n*2 + *retries
}
`

func TestArid_TimeValuesAndFlags(t *testing.T) {
	t.Parallel()

	kept, left := aridLines(t, aridTimeAndFlags, aridOn, mgen.Arithmetic, mgen.Numbers)
	assert.Equal(t, []int{17, 22}, kept, "the function a timer runs and the result stay")
	assert.Equal(t, []int{10, 13, 15, 16, 17, 19}, left)
}

func TestArid_EveryTimeCall(t *testing.T) {
	t.Parallel()

	const src = `package p

import (
	"context"
	"net"
	"time"
)

func timers(ctx context.Context, conn net.Conn, f func(int)) {
	_, _ = context.WithDeadline(ctx, time.Now().Add(2*time.Second))
	_, _ = context.WithTimeout(ctx, 2*time.Second)
	_ = time.After(2 * time.Second)
	_ = time.AfterFunc(2*time.Second, func() {
		f(1 + 1)
	})
	_ = time.NewTicker(2 * time.Second)
	_ = time.NewTimer(2 * time.Second)
	time.Sleep(2 * time.Second)
	_ = time.Tick(2 * time.Second)
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_ = time.Duration(2 * 3)
	_, _ = context.WithTimeout()
	time.Sleep()
	_ = conn.SetDeadline()
	_ = context.WithTimeout(2 * 4)
}
`

	kept, left := aridLines(t, src, aridOn, mgen.Arithmetic)
	assert.Equal(t, []int{14, 23, 27}, kept, "the function of a timer, a plain duration and a call without its time argument stay")
	assert.Equal(t, []int{10, 11, 12, 13, 16, 17, 18, 19, 20, 21, 22}, left)
}

func TestArid_RulesSelectTheFamilies(t *testing.T) {
	t.Parallel()

	kept, left := aridLines(t, aridTimeAndFlags, domain.AridOptions{Enabled: true, Rules: []string{"flags"}}, mgen.Arithmetic, mgen.Numbers)
	assert.Equal(t, []int{13, 15, 16, 17, 19, 22}, kept)
	assert.Equal(t, []int{10}, left)

	kept, left = aridLines(t, aridTimeAndFlags, domain.AridOptions{Enabled: true, Rules: []string{"time"}}, mgen.Arithmetic, mgen.Numbers)
	assert.Equal(t, []int{10, 17, 22}, kept)
	assert.Equal(t, []int{13, 15, 16, 17, 19}, left)

	kept, _ = aridLines(t, aridLogging, domain.AridOptions{Enabled: true, Rules: []string{"time", "flags", "metrics"}}, mgen.Arithmetic)
	assert.Contains(t, kept, 11, "logging is not among the rules")
}

func TestArid_MetricsAndChains(t *testing.T) {
	t.Parallel()

	const src = `package p

import (
	"github.com/prometheus/client_golang/prometheus"
	zl "github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var requests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "requests"}, []string{"code"})

type catalog struct{ items int }

func (c *catalog) Add(n int) { c.items += n }

func serve(cmd *cobra.Command, c *catalog, hist prometheus.Histogram, n int) int {
	requests.WithLabelValues("ok").Add(float64(n + 1))
	hist.Observe(float64(n) * 0.5)
	zl.Info().Int("n", n+2).Msg("served")
	cmd.Flags().Int("port", 8000+80, "port to listen on")
	c.Add(n + 3)
	defer zl.Debug().Int("n", n+5).Msg("done")
	return n + 4
}
`

	kept, left := aridLines(t, src, aridOn, mgen.Arithmetic)
	assert.Equal(t, []int{20, 22}, kept, "a method of the code's own and the result stay")
	assert.Equal(t, []int{16, 17, 18, 19, 21}, left)
}

func TestArid_CallsAddPatterns(t *testing.T) {
	t.Parallel()

	const src = `package p

import "example.com/app/audit"

type tracer struct{}

func (tracer) Tracef(string, ...any) {}

func f(tr tracer, n int) int {
	audit.Record("f", n+1)
	tr.Tracef("n=%d", n+2)
	return n + 3
}
`

	kept, left := aridLines(t, src, aridOn, mgen.Arithmetic)
	assert.Equal(t, []int{10, 11, 12}, kept)
	assert.Empty(t, left)

	kept, left = aridLines(t, src, domain.AridOptions{Enabled: true, Calls: []string{"example.com/app/audit.*", "*.Tracef", "nodot"}}, mgen.Arithmetic)
	assert.Equal(t, []int{12}, kept)
	assert.Equal(t, []int{10, 11}, left)

	kept, _ = aridLines(t, src, domain.AridOptions{Enabled: true, Calls: []string{"audit.Record"}}, mgen.Arithmetic)
	assert.Equal(t, []int{11, 12}, kept, "a package matches by its name too")

	kept, _ = aridLines(t, src, domain.AridOptions{Enabled: true, Calls: []string{"audit.Other", "other.Record"}}, mgen.Arithmetic)
	assert.Equal(t, []int{10, 11, 12}, kept)

	kept, _ = aridLines(t, src, domain.AridOptions{Enabled: true, Calls: []string{"Record", "audit.", ".Tracef", "aTracef"}}, mgen.Arithmetic)
	assert.Equal(t, []int{10, 11, 12}, kept, "a pattern needs a receiver and a function")
}

func TestArid_OnlyCallsThroughASelector(t *testing.T) {
	t.Parallel()

	const src = `package p

import (
	"log"
	"log/slog"

	"k8s.io/klog/v2"
)

func record(int) {}

func count() int { return 0 }

func f(n int, done chan int, fns []func(int)) int {
	record(n + 1)
	fns[0](n + 2)
	func(int) {}(n + 3)
	klog.Infof("n=%d", n+4)
	if n > 5 {
		<-done
	}
	if n > 6 {
		record(n)
	}
	total := slog.Default().Handler().Enabled(nil, 0)||n+7 > 0
	log.Panicf("n=%d", n+8)
	slog.Default().With("n", n+9).Info("chained")
	return count()+n
}
`

	kept, left := aridLines(t, src, aridOn, mgen.Arithmetic, mgen.Comparison, mgen.Logical)
	assert.Equal(t, []int{15, 16, 17, 19, 22, 25, 26, 28}, kept,
		"plain calls, conditions over them or a receive, what follows a call, and a log that panics stay")
	assert.Equal(t, []int{18, 27}, left)
}

func TestArid_PropagatesToConditions(t *testing.T) {
	t.Parallel()

	const src = `package p

import "log"

func f(n int, err error) int {
	if n > 1 {
		log.Print("a")
	} else {
		log.Print("b")
	}
	if n > 2 {
		log.Print("a")
	} else {
		n++
	}
	if n > 3 {
		if n > 4 {
			log.Print("deep")
		}
		{
			log.Print("block")
		}
	}
	if n > 5 {
	}
	if m := n * 2; m > 6 {
		log.Print("init")
	}
	if n > 7 {
		log.Print("a")
	} else if n > 8 {
		log.Print("b")
	}
	if n > 9 {
		log.Print("a")
	} else if n > 10 {
		return n
	}
	for n > 11 {
		log.Print("loop")
	}
	return n
}
`

	kept, left := aridLines(t, src, aridOn, mgen.Comparison, mgen.Arithmetic)
	assert.Equal(t, []int{11, 24, 26, 34, 36, 39}, kept,
		"a branch that does work, an empty body, an init statement and a loop keep their conditions")
	assert.Equal(t, []int{6, 16, 17, 26, 29, 31}, left)
}

func TestArid_DirectiveReasonWins(t *testing.T) {
	t.Parallel()

	const src = `package p

import "log"

func f(n int) {
	log.Print(n + 1) //gooze:ignore arithmetic reason=deferred
	log.Print(n + 2)
}
`

	assert.Equal(t, []string{
		"6 arithmetic deferred", "6 arithmetic deferred", "6 arithmetic deferred", "6 arithmetic deferred",
	}, ignoredIn(t, src, mgen.Arithmetic), "without arid only the directive ignores")

	path := filepath.Join(t.TempDir(), "p.go")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

	mg, ok := newTestMutagen().(domain.MutagenWithOptions)
	require.True(t, ok)

	reasons := map[int]string{}

	err := mg.StreamMutationsWith(context.Background(), makeSourceV2(t, path), domain.MutationOptions{Arid: aridOn, Ignored: true}, func(mutation m.Mutation) error {
		reasons[mutation.Line] = mutation.Ignored.Reason

		return nil
	}, mgen.Arithmetic)
	require.NoError(t, err)
	assert.Equal(t, map[int]string{6: "deferred", 7: domain.AridReason}, reasons)
}

func TestWorkflow_IgnoredListsAridMutants(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/arid\n\ngo 1.24\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte(`package arid

import "log"

func f(n int) int {
	log.Print(n + 1)
	return n
}
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a_test.go"), []byte("package arid\n"), 0o600))
	t.Chdir(dir)

	args := domain.EstimateArgs{Paths: []m.Path{"./..."}, Mutagens: []string{"arithmetic"}}
	assert.Equal(t, "No mutants are ignored.\n", viewIgnored(t, args))

	args.Arid = aridOn
	assert.Equal(t, `Ignored (4):
a.go:6: arithmetic (arid): log.Print(n + 1) -> log.Print(n - 1)
a.go:6: arithmetic (arid): log.Print(n + 1) -> log.Print(n * 1)
a.go:6: arithmetic (arid): log.Print(n + 1) -> log.Print(n / 1)
a.go:6: arithmetic (arid): log.Print(n + 1) -> log.Print(n % 1)

By reason: arid 4
By mutagen: arithmetic 4
`, viewIgnored(t, args))
}

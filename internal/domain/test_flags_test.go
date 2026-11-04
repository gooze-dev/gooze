package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "gooze.dev/pkg/gooze/internal/domain"
)

func TestParseTestFlags(t *testing.T) {
	cases := map[string]struct {
		entries []string
		want    []string
		err     string
	}{
		"none":                  {},
		"one per entry":         {entries: []string{"-short", "-race"}, want: []string{"-short", "-race"}},
		"several in one entry":  {entries: []string{"-short  -timeout=30s"}, want: []string{"-short", "-timeout=30s"}},
		"double dash":           {entries: []string{"--tags=integration"}, want: []string{"--tags=integration"}},
		"blank entries":         {entries: []string{"", "  "}},
		"a value needs =":       {entries: []string{"-skip TestSlow"}, err: `run.test_flags: "TestSlow" is not a flag; write a flag's value with =, as in -skip=TestSlow`},
		"a package":             {entries: []string{"./..."}, err: `run.test_flags: "./..." is not a flag`},
		"gooze's own -json":     {entries: []string{"-json"}, err: "run.test_flags: gooze sets -json itself or can't run with it"},
		"gooze's own -count":    {entries: []string{"-count=2"}, err: "run.test_flags: gooze sets -count itself or can't run with it"},
		"gooze's own -overlay":  {entries: []string{"--overlay=o.json"}, err: "run.test_flags: gooze sets -overlay itself or can't run with it"},
		"gooze's own -vet":      {entries: []string{"-vet=all"}, err: "run.test_flags: gooze sets -vet itself or can't run with it"},
		"no test binary output": {entries: []string{"-c"}, err: "run.test_flags: gooze sets -c itself or can't run with it"},
		"no output file":        {entries: []string{"-o=x.test"}, err: "run.test_flags: gooze sets -o itself or can't run with it"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			flags, err := domain.ParseTestFlags(tc.entries)
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, flags)
		})
	}
}

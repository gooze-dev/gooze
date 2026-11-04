package report

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	"gooze.dev/pkg/gooze/pkg/config"
)

// expectPush returns a publisher that expects one push whose OCI compat
// settings are compat (nil: automatic) and hosts.
func expectPush(t *testing.T, compat *bool, hosts []string) *domainmocks.MockReportPublisher {
	t.Helper()

	pub := domainmocks.NewMockReportPublisher(t)
	pub.On("Push", mock.Anything, mock.MatchedBy(func(args domain.PushArgs) bool {
		return reflect.DeepEqual(args.OCICompat, compat) && slices.Equal(args.OCICompatHosts, hosts)
	})).Return(nil)

	return pub
}

// readConfig loads yaml as gooze.yaml.
func readConfig(t *testing.T, yaml string) {
	t.Helper()

	viper.SetConfigType("yaml")
	require.NoError(t, viper.ReadConfig(strings.NewReader(yaml)))
}

func TestReportPush_OCICompat(t *testing.T) {
	on, off := true, false

	cases := map[string]struct {
		env  string // GOOZE_REPORT_OCI_COMPAT
		yaml string // gooze.yaml
		args []string
		want *bool
	}{
		"unset: automatic":           {},
		"gooze.yaml true":            {yaml: "report:\n  oci:\n    compat: true\n", want: &on},
		"gooze.yaml false":           {yaml: "report:\n  oci:\n    compat: false\n", want: &off},
		"env true":                   {env: "true", want: &on},
		"env false":                  {env: "false", want: &off},
		"env over gooze.yaml":        {env: "false", yaml: "report:\n  oci:\n    compat: true\n", want: &off},
		"flag":                       {args: []string{"--oci-compat"}, want: &on},
		"flag false over gooze.yaml": {args: []string{"--oci-compat=false"}, yaml: "report:\n  oci:\n    compat: true\n", want: &off},
		"flag over env":              {args: []string{"--oci-compat"}, env: "false", want: &on},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			defer viper.Reset()

			viper.SetEnvPrefix("GOOZE")
			viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
			viper.AutomaticEnv()
			t.Setenv("GOOZE_REPORT_OCI_COMPAT", tc.env)

			if tc.yaml != "" {
				readConfig(t, tc.yaml)
			}

			viper.Set(testOutputKey, "/tmp/reports")

			pub := expectPush(t, tc.want, nil)
			args := append(append([]string{"push"}, tc.args...), "registry.gitlab.com/g/p:main")

			require.NoError(t, execReport(t, Deps{Publisher: pub, OutputKey: testOutputKey}, args...))
		})
	}
}

func TestReportPush_OCICompatHostsFromConfig(t *testing.T) {
	defer viper.Reset()

	readConfig(t, "report:\n  oci:\n    compat_hosts: [registry.example.com]\n")
	viper.Set(testOutputKey, "/tmp/reports")

	pub := expectPush(t, nil, []string{"registry.example.com"})

	require.NoError(t, execReport(t, Deps{Publisher: pub, OutputKey: testOutputKey}, "push", "registry.example.com/p:main"))
}

func TestReportPush_OCICompatFlagDefaultsOff(t *testing.T) {
	cmd := newPushCmd(Deps{})

	flag := cmd.Flags().Lookup(ociCompatFlagName)
	require.NotNil(t, flag)
	require.Equal(t, "false", flag.DefValue, "--help must not say compat is on for every registry")
}

func TestReportPush_OCICompatWithTheConfigInitFile(t *testing.T) {
	defer viper.Reset()

	viper.SetConfigType("yaml")
	require.NoError(t, viper.ReadConfig(bytes.NewReader(config.DefaultFile())))
	viper.Set(testOutputKey, "/tmp/reports")

	pub := expectPush(t, nil, []string{})

	require.NoError(t, execReport(t, Deps{Publisher: pub, OutputKey: testOutputKey}, "push", "registry.gitlab.com/g/p:main"),
		"the file gooze config init writes leaves compat automatic")
}

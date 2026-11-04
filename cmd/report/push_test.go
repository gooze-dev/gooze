package report

import (
	"slices"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gooze.dev/pkg/gooze/internal/domain"
	domainmocks "gooze.dev/pkg/gooze/internal/domain/mocks"
	"gooze.dev/pkg/gooze/pkg/config"
)

// expectPush returns a publisher that expects one push whose OCI compat
// settings are compat and hosts.
func expectPush(t *testing.T, compat bool, hosts []string) *domainmocks.MockReportPublisher {
	t.Helper()

	pub := domainmocks.NewMockReportPublisher(t)
	pub.On("Push", mock.Anything, mock.MatchedBy(func(args domain.PushArgs) bool {
		return args.OCICompat == compat && slices.Equal(args.OCICompatHosts, hosts)
	})).Return(nil)

	return pub
}

func TestReportPush_OCICompatDefaultsOff(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	defer viper.Reset()

	pub := expectPush(t, false, nil)

	require.NoError(t, execReport(t, Deps{Publisher: pub, OutputKey: testOutputKey}, "push", "ghcr.io/org/repo:tag"))
}

func TestReportPush_OCICompatFromConfig(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	viper.Set(config.KeyReportOCICompat, true)
	viper.Set(config.KeyReportOCICompatHosts, []string{"registry.example.com"})
	defer viper.Reset()

	pub := expectPush(t, true, []string{"registry.example.com"})

	require.NoError(t, execReport(t, Deps{Publisher: pub, OutputKey: testOutputKey}, "push", "ghcr.io/org/repo:tag"))
}

func TestReportPush_OCICompatFlagOverridesConfig(t *testing.T) {
	viper.Set(testOutputKey, "/tmp/reports")
	viper.Set(config.KeyReportOCICompat, true)
	defer viper.Reset()

	pub := expectPush(t, false, nil)

	require.NoError(t, execReport(t, Deps{Publisher: pub, OutputKey: testOutputKey}, "push", "--oci-compat=false", "ghcr.io/org/repo:tag"))

	viper.Set(config.KeyReportOCICompat, false)

	pub = expectPush(t, true, nil)

	require.NoError(t, execReport(t, Deps{Publisher: pub, OutputKey: testOutputKey}, "push", "--oci-compat", "ghcr.io/org/repo:tag"))
}

func TestReportPush_OCICompatFlagDefaultsOff(t *testing.T) {
	cmd := newPushCmd(Deps{})

	flag := cmd.Flags().Lookup(ociCompatFlagName)
	require.NotNil(t, flag)
	require.Equal(t, "false", flag.DefValue, "--help must not say compat is on for every registry")
}

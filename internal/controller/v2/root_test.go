package v2

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRoot(t *testing.T) {
	t.Run("no args", func(t *testing.T) {
		cmd := &cobra.Command{
			Use: "gooze",
		}
		err := Root(cmd, []string{})
		require.NoError(t, err)
	})

	t.Run("with args", func(t *testing.T) {
		cmd := &cobra.Command{
			Use: "gooze",
		}
		err := Root(cmd, []string{"unexpected"})
		require.NoError(t, err)
	})

	t.Run("Output contains help message", func(t *testing.T) {
		cmd := &cobra.Command{
			Use: "gooze",
		}
		err := Root(cmd, []string{"--help"})
		require.NoError(t, err)
	})
}

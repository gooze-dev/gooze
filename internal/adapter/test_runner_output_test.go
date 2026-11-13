package adapter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOutputBuffer_KeepsShortOutputWhole(t *testing.T) {
	var buf outputBuffer

	require.Empty(t, buf.String())

	for _, part := range []string{"=== RUN   TestA\n", "", "--- PASS: TestA (0.00s)\n", "ok\n"} {
		n, err := buf.Write([]byte(part))
		require.NoError(t, err)
		require.Equal(t, len(part), n)
	}

	require.Equal(t, "=== RUN   TestA\n--- PASS: TestA (0.00s)\nok\n", buf.String())
}

func TestOutputBuffer_KeepsAllUpToTheLimit(t *testing.T) {
	var buf outputBuffer

	whole := strings.Repeat("a", outputKeep) + strings.Repeat("z", outputKeep)

	_, err := buf.Write([]byte(whole))
	require.NoError(t, err)
	require.Equal(t, whole, buf.String(), "the start and the end, nothing between them")
}

func TestOutputBuffer_DropsTheMiddle(t *testing.T) {
	for name, chunk := range map[string]int{"one write": 0, "small writes": 7, "writes of the kept size": outputKeep, "large writes": 3*outputKeep + 1} {
		t.Run(name, func(t *testing.T) {
			var buf outputBuffer

			start := strings.Repeat("a", outputKeep)
			middle := strings.Repeat("m", 5*outputKeep+3)
			end := strings.Repeat("z", outputKeep-1) + "\n"
			whole := start + middle + end

			if chunk == 0 {
				chunk = len(whole)
			}

			for lo := 0; lo < len(whole); lo += chunk {
				part := whole[lo:min(lo+chunk, len(whole))]

				n, err := buf.Write([]byte(part))
				require.NoError(t, err)
				require.Equal(t, len(part), n, "the writer is never told that output was dropped")
			}

			require.Equal(t, start+"\n... gooze dropped 5242883 bytes of output ...\n"+end, buf.String())
		})
	}
}

func TestOutputBuffer_DropsOneByte(t *testing.T) {
	var buf outputBuffer

	_, err := buf.Write([]byte(strings.Repeat("a", outputKeep) + "m" + strings.Repeat("z", outputKeep)))
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("a", outputKeep)+"\n... gooze dropped 1 bytes of output ...\n"+strings.Repeat("z", outputKeep), buf.String())
}

func TestOutputBuffer_KeepsSingleBytesAfterTheStart(t *testing.T) {
	var buf outputBuffer

	start := strings.Repeat("a", outputKeep)

	for _, part := range []string{start, "x", "y", ""} {
		_, err := buf.Write([]byte(part))
		require.NoError(t, err)
	}

	require.Equal(t, start+"xy", buf.String())
}

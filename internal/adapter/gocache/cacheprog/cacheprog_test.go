package cacheprog

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requests encodes requests the way the go command sends them: each JSON
// object on its own line, a put's body after it as a base64 JSON string.
func requests(t *testing.T, reqs ...request) *bytes.Buffer {
	t.Helper()

	var in bytes.Buffer
	for _, req := range reqs {
		require.NoError(t, json.NewEncoder(&in).Encode(req))
		in.WriteString("\n")

		if req.BodySize > 0 {
			in.WriteString(`"` + base64.StdEncoding.EncodeToString(make([]byte, req.BodySize)) + "\"\n")
		}
	}

	return &in
}

func serve(t *testing.T, in io.Reader, shared, layerDir string) []response {
	t.Helper()

	var out bytes.Buffer
	require.NoError(t, Serve(in, &out, shared, layerDir))

	var responses []response
	for dec := json.NewDecoder(&out); dec.More(); {
		var res response
		require.NoError(t, dec.Decode(&res))
		responses = append(responses, res)
	}

	return responses
}

func TestServe_AnnouncesCommands(t *testing.T) {
	got := serve(t, strings.NewReader(""), t.TempDir(), t.TempDir())
	assert.Equal(t, []response{{KnownCommands: []string{"get", "put", "close"}}}, got)
}

func TestServe_PutThenGet(t *testing.T) {
	layerDir := t.TempDir()
	in := requests(t,
		request{ID: 1, Command: "get", ActionID: testAction[:]},
		request{ID: 2, Command: "put", ActionID: testAction[:], OutputID: testOutput[:], BodySize: 3},
		request{ID: 3, Command: "get", ActionID: testAction[:]},
		request{ID: 4, Command: "close"},
		request{ID: 5, Command: "get", ActionID: testAction[:]},
	)

	got := serve(t, in, t.TempDir(), layerDir)
	require.Len(t, got, 5, "nothing after close")

	path := cacheFile(layerDir, testOutput[:], "d")
	assert.Equal(t, response{ID: 1, Miss: true}, got[1])
	assert.Equal(t, response{ID: 2, DiskPath: path}, got[2])
	assert.Equal(t, int64(3), got[3].ID)
	assert.Equal(t, testOutput[:], got[3].OutputID)
	assert.Equal(t, int64(3), got[3].Size)
	assert.Equal(t, path, got[3].DiskPath)
	require.NotNil(t, got[3].Time)
	assert.Equal(t, response{ID: 4}, got[4])
}

func TestServe_PutWithoutBody(t *testing.T) {
	layerDir := t.TempDir()
	in := requests(t,
		request{ID: 1, Command: "put", ActionID: testAction[:], OutputID: testOutput[:]},
		request{ID: 2, Command: "get", ActionID: testAction[:]},
	)

	got := serve(t, in, t.TempDir(), layerDir)
	require.Len(t, got, 3)
	assert.Equal(t, int64(0), got[2].Size)
	assert.Equal(t, cacheFile(layerDir, testOutput[:], "d"), got[2].DiskPath)
}

func TestServe_ReadsThroughShared(t *testing.T) {
	shared, layerDir := t.TempDir(), t.TempDir()
	writeEntry(t, shared, testAction[:], testOutput[:], "body")

	got := serve(t, requests(t, request{ID: 1, Command: "get", ActionID: testAction[:]}), shared, layerDir)
	require.Len(t, got, 2)
	assert.Equal(t, cacheFile(shared, testOutput[:], "d"), got[1].DiskPath)
	assert.Equal(t, int64(4), got[1].Size)
	assert.True(t, got[1].Time.Equal(testTime))
}

func TestServe_LayerFirst(t *testing.T) {
	shared, layerDir := t.TempDir(), t.TempDir()
	writeEntry(t, shared, testAction[:], testOutput[:], "body")
	writeEntry(t, layerDir, testAction[:], testOutput[:], "body")

	got := serve(t, requests(t, request{ID: 1, Command: "get", ActionID: testAction[:]}), shared, layerDir)
	require.Len(t, got, 2)
	assert.Equal(t, cacheFile(layerDir, testOutput[:], "d"), got[1].DiskPath)
}

func TestServe_Errors(t *testing.T) {
	layerFile := filepath.Join(t.TempDir(), "file")
	writeTestFile(t, layerFile, "")

	in := requests(t,
		request{ID: 1, Command: "get2"},
		request{ID: 2, Command: "put", ActionID: testAction[:], OutputID: testOutput[:], BodySize: 1},
	)

	got := serve(t, in, t.TempDir(), layerFile)
	require.Len(t, got, 3)
	assert.Equal(t, response{ID: 1, Err: `unknown command "get2"`}, got[1])
	assert.Equal(t, int64(2), got[2].ID)
	assert.NotEmpty(t, got[2].Err)
	assert.Empty(t, got[2].DiskPath)
}

func TestServe_BadInput(t *testing.T) {
	// The writer's limit ends a server that doesn't stop at the bad input.
	out := &failingWriter{after: 10}
	require.Error(t, Serve(strings.NewReader("{"), out, t.TempDir(), t.TempDir()))

	in := requests(t, request{ID: 1, Command: "put", ActionID: testAction[:], OutputID: testOutput[:]})
	in.WriteString(`{"ID":2,"Command":"put","BodySize":1}` + "\n" + `"!"` + "\n")
	require.Error(t, Serve(in, &failingWriter{after: 10}, t.TempDir(), t.TempDir()), "body isn't base64")
}

type failingWriter struct{ after int }

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.after == 0 {
		return 0, errors.New("closed")
	}

	w.after--

	return len(p), nil
}

func TestServe_WriteErrors(t *testing.T) {
	require.Error(t, Serve(strings.NewReader(""), &failingWriter{}, t.TempDir(), t.TempDir()))

	in := requests(t, request{ID: 1, Command: "close"})
	require.Error(t, Serve(in, &failingWriter{after: 1}, t.TempDir(), t.TempDir()))
}

func TestServe_OnlyCloseEndsTheSession(t *testing.T) {
	in := requests(t,
		request{ID: 1, Command: "a"},
		request{ID: 2, Command: "get", ActionID: testAction[:]},
	)

	got := serve(t, in, t.TempDir(), t.TempDir())
	require.Len(t, got, 3, "a command sorted before close doesn't end it")
	assert.Equal(t, response{ID: 2, Miss: true}, got[2])
}

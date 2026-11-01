package cacheprog

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// The commands gooze's server knows.
const (
	cmdGet   = "get"
	cmdPut   = "put"
	cmdClose = "close"
)

// request and response are the messages of the GOCACHEPROG protocol, as
// cmd/go/internal/cacheprog defines them.
type request struct {
	ID       int64
	Command  string
	ActionID []byte `json:",omitempty"`
	OutputID []byte `json:",omitempty"`
	BodySize int64  `json:",omitempty"`
}

type response struct {
	ID            int64
	Err           string     `json:",omitempty"`
	KnownCommands []string   `json:",omitempty"`
	Miss          bool       `json:",omitempty"`
	OutputID      []byte     `json:",omitempty"`
	Size          int64      `json:",omitempty"`
	Time          *time.Time `json:",omitempty"`
	DiskPath      string     `json:",omitempty"`
}

// Serve answers the go command's GOCACHEPROG requests from in on out until
// in ends or a close request. Gets read layerDir, then shared; puts write to
// layerDir only.
func Serve(in io.Reader, out io.Writer, shared, layerDir string) error {
	dec := json.NewDecoder(bufio.NewReader(in))
	buf := bufio.NewWriter(out)
	enc := json.NewEncoder(buf)
	send := func(res response) error {
		return errors.Join(enc.Encode(res), buf.Flush())
	}

	if err := send(response{KnownCommands: []string{cmdGet, cmdPut, cmdClose}}); err != nil {
		return err
	}

	for dec.More() {
		var req request
		if err := dec.Decode(&req); err != nil {
			return err
		}

		res, err := answer(dec, req, shared, Layer{Dir: layerDir})
		if err != nil {
			return err
		}

		if err := send(res); err != nil || req.Command == cmdClose {
			return err
		}
	}

	return nil
}

// answer handles req. A put's body is the next value of dec.
func answer(dec *json.Decoder, req request, shared string, l Layer) (response, error) {
	res := response{ID: req.ID}

	switch req.Command {
	case cmdGet:
		e, ok := l.get(req.ActionID)
		if !ok {
			e, ok = readEntry(shared, req.ActionID)
		}

		if !ok {
			res.Miss = true
			return res, nil
		}

		res.OutputID, res.Size, res.Time, res.DiskPath = e.outputID, e.size, &e.time, e.path
	case cmdPut:
		var body []byte
		// Equivalent: the go command never sends a negative size, so > 0 is != 0.
		if req.BodySize > 0 { //gooze:ignore comparison
			if err := dec.Decode(&body); err != nil {
				return res, err
			}
		}

		path, err := l.put(req.ActionID, req.OutputID, body, time.Now())
		if err != nil {
			res.Err = err.Error()
		}

		res.DiskPath = path
	case cmdClose:
	default:
		res.Err = fmt.Sprintf("unknown command %q", req.Command)
	}

	return res, nil
}

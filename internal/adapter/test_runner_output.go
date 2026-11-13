package adapter

import "strconv"

// outputKeep is how many bytes of the start and of the end of a stream are
// kept.
const outputKeep = 1 << 20

// outputBuffer keeps what a go test writes to one stream: all of it up to
// twice outputKeep, and its start and its end beyond that. Tests that print
// without end would otherwise fill gooze's memory, while what gooze reads is
// at the start (the first failure) and at the end (the verdict).
type outputBuffer struct {
	head []byte
	// ring holds the last outputKeep bytes written after head; next is where
	// the following byte goes, and after counts the bytes written after head.
	ring  []byte
	next  int
	after int
}

// Write implements io.Writer. It never fails and never writes short.
func (b *outputBuffer) Write(p []byte) (int, error) {
	written := len(p)

	taken := min(outputKeep-len(b.head), len(p))
	b.head = append(b.head, p[:taken]...)
	p = p[taken:]
	b.after += len(p)

	// Of a write longer than the ring only the end stays.
	p = p[max(0, len(p)-outputKeep):]

	if b.ring == nil {
		b.ring = make([]byte, outputKeep)
	}

	// Up to the end of the ring, then the rest from its start.
	copied := copy(b.ring[b.next:], p)
	copy(b.ring, p[copied:])
	b.next = (b.next + len(p)) % outputKeep

	return written, nil
}

// String returns what was kept, with a line in place of what was dropped.
func (b *outputBuffer) String() string {
	if b.after <= outputKeep {
		return string(b.head) + string(b.ring[:b.after])
	}

	dropped := "\n... gooze dropped " + strconv.Itoa(b.after-outputKeep) + " bytes of output ...\n"

	return string(b.head) + dropped + string(b.ring[b.next:]) + string(b.ring[:b.next])
}

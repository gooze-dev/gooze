package adapter

import "strconv"

const outputKeep = 1 << 20

type outputBuffer struct {
	head  []byte
	ring  []byte
	next  int
	after int
}

func (b *outputBuffer) Write(p []byte) (int, error) {
	written := len(p)

	taken := min(outputKeep-len(b.head), len(p))
	b.head = append(b.head, p[:taken]...)
	p = p[taken:]
	b.after += len(p)

	p = p[max(0, len(p)-outputKeep):]

	if b.ring == nil {
		b.ring = make([]byte, outputKeep)
	}

	copied := copy(b.ring[b.next:], p)
	copy(b.ring, p[copied:])
	b.next = (b.next + len(p)) % outputKeep

	return written, nil
}

func (b *outputBuffer) String() string {
	if b.after <= outputKeep {
		return string(b.head) + string(b.ring[:b.after])
	}

	dropped := "\n... gooze dropped " + strconv.Itoa(b.after-outputKeep) + " bytes of output ...\n"

	return string(b.head) + dropped + string(b.ring[b.next:]) + string(b.ring[:b.next])
}

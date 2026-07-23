package termimage

import (
	"io"
	"sync"
)

type SealedEncodedPayload struct {
	header Header
	epoch  StoreEpoch
	chunks [][]byte
	leases []*reservation
	base   *reservation
	once   sync.Once
}

func (t *CandidateTransfer) TakeSealedPayload(store *Store) (*SealedEncodedPayload, error) {
	if t == nil || store == nil {
		return nil, ErrTransferClosed
	}
	t.mu.Lock()
	if t.closing.Load() || t.open || t.store != store || store.closed.Load() || store.resetting.Load() || t.epoch != StoreEpoch(store.epoch.Load()) || !t.closing.CompareAndSwap(false, true) {
		t.mu.Unlock()
		return nil, ErrTransferClosed
	}
	payload := &SealedEncodedPayload{header: t.header, epoch: t.epoch, chunks: t.chunks, leases: t.leases, base: t.base}
	t.chunks, t.leases, t.base, t.encoded = nil, nil, nil, 0
	t.mu.Unlock()
	store.removeTransfer(t)
	return payload, nil
}

func (p *SealedEncodedPayload) Header() Header {
	if p == nil {
		return Header{}
	}
	return p.header
}

func (p *SealedEncodedPayload) Epoch() StoreEpoch {
	if p == nil {
		return 0
	}
	return p.epoch
}

func (p *SealedEncodedPayload) EncodedLen() uint64 {
	var total uint64
	if p != nil {
		for _, chunk := range p.chunks {
			total += uint64(len(chunk))
		}
	}
	return total
}

func (p *SealedEncodedPayload) Reader() io.Reader {
	if p == nil {
		return &encodedChunkReader{}
	}
	return &encodedChunkReader{chunks: p.chunks}
}

func (p *SealedEncodedPayload) Close() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		for i := len(p.leases) - 1; i >= 0; i-- {
			p.leases[i].Close()
		}
		if p.base != nil {
			p.base.Close()
		}
		p.chunks, p.leases, p.base = nil, nil, nil
	})
}

type encodedChunkReader struct {
	chunks        [][]byte
	chunk, offset int
}

func (r *encodedChunkReader) Read(dst []byte) (int, error) {
	for r.chunk < len(r.chunks) {
		if r.offset == len(r.chunks[r.chunk]) {
			r.chunk++
			r.offset = 0
			continue
		}
		n := copy(dst, r.chunks[r.chunk][r.offset:])
		r.offset += n
		return n, nil
	}
	return 0, io.EOF
}

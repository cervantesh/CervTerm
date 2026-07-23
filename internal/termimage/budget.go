package termimage

import "sync/atomic"

type budgetCounters struct {
	encoded    atomic.Uint64
	decoded    atomic.Uint64
	images     atomic.Uint64
	placements atomic.Uint64
	transfers  atomic.Uint64
}

type ProcessBudget struct {
	counters budgetCounters
}

func NewProcessBudget() *ProcessBudget { return &ProcessBudget{} }

func (b *ProcessBudget) Usage() Usage {
	if b == nil {
		return Usage{}
	}
	return b.counters.usage()
}

type paneBudget struct {
	counters budgetCounters
	limits   Limits
}

func (b *paneBudget) usage() Usage { return b.counters.usage() }

func (c *budgetCounters) usage() Usage {
	return Usage{
		EncodedBytes:     c.encoded.Load(),
		DecodedBytes:     c.decoded.Load(),
		Images:           c.images.Load(),
		Placements:       c.placements.Load(),
		PendingTransfers: c.transfers.Load(),
	}
}

type reservation struct {
	process *ProcessBudget
	pane    *paneBudget
	usage   Usage
	closed  atomic.Bool
}

func reserve(process *ProcessBudget, pane *paneBudget, usage Usage) (*reservation, error) {
	if process == nil || pane == nil {
		return nil, ErrInvalidLimits
	}
	acquired := Usage{}
	if !reservePair(&pane.counters.encoded, &process.counters.encoded, pane.limits.EncodedBytes, HardEncodedBytesProcess, usage.EncodedBytes) {
		return nil, ErrLimitExceeded
	}
	acquired.EncodedBytes = usage.EncodedBytes
	if !reservePair(&pane.counters.decoded, &process.counters.decoded, pane.limits.DecodedBytes, HardDecodedBytesProcess, usage.DecodedBytes) {
		releaseUsage(process, pane, acquired)
		return nil, ErrLimitExceeded
	}
	acquired.DecodedBytes = usage.DecodedBytes
	if !reservePair(&pane.counters.images, &process.counters.images, pane.limits.Images, HardImagesProcess, usage.Images) {
		releaseUsage(process, pane, acquired)
		return nil, ErrLimitExceeded
	}
	acquired.Images = usage.Images
	if !reservePair(&pane.counters.placements, &process.counters.placements, pane.limits.Placements, HardPlacementsProcess, usage.Placements) {
		releaseUsage(process, pane, acquired)
		return nil, ErrLimitExceeded
	}
	acquired.Placements = usage.Placements
	if !reservePair(&pane.counters.transfers, &process.counters.transfers, HardPendingTransfersPerPane, HardPendingTransfersProcess, usage.PendingTransfers) {
		releaseUsage(process, pane, acquired)
		return nil, ErrLimitExceeded
	}
	acquired.PendingTransfers = usage.PendingTransfers
	return &reservation{process: process, pane: pane, usage: acquired}, nil
}

func reservePair(pane, process *atomic.Uint64, paneLimit, processLimit, amount uint64) bool {
	if amount == 0 {
		return true
	}
	if !reserveCounter(pane, paneLimit, amount) {
		return false
	}
	if !reserveCounter(process, processLimit, amount) {
		releaseCounter(pane, amount)
		return false
	}
	return true
}

func reserveCounter(counter *atomic.Uint64, limit, amount uint64) bool {
	if amount > limit {
		return false
	}
	for {
		current := counter.Load()
		if current > limit || amount > limit-current {
			return false
		}
		if counter.CompareAndSwap(current, current+amount) {
			return true
		}
	}
}

func (r *reservation) Close() {
	if r == nil || !r.closed.CompareAndSwap(false, true) {
		return
	}
	releaseUsage(r.process, r.pane, r.usage)
}

func releaseUsage(process *ProcessBudget, pane *paneBudget, usage Usage) {
	releaseCounter(&process.counters.transfers, usage.PendingTransfers)
	releaseCounter(&pane.counters.transfers, usage.PendingTransfers)
	releaseCounter(&process.counters.placements, usage.Placements)
	releaseCounter(&pane.counters.placements, usage.Placements)
	releaseCounter(&process.counters.images, usage.Images)
	releaseCounter(&pane.counters.images, usage.Images)
	releaseCounter(&process.counters.decoded, usage.DecodedBytes)
	releaseCounter(&pane.counters.decoded, usage.DecodedBytes)
	releaseCounter(&process.counters.encoded, usage.EncodedBytes)
	releaseCounter(&pane.counters.encoded, usage.EncodedBytes)
}

func releaseCounter(counter *atomic.Uint64, amount uint64) {
	if amount == 0 {
		return
	}
	for {
		current := counter.Load()
		if current < amount {
			panic("termimage reservation underflow")
		}
		if counter.CompareAndSwap(current, current-amount) {
			return
		}
	}
}

package broker

import (
	"log/slog"
	"sync/atomic"
)

// Paper is the paper-book broker. Admit is the only buy path the desk may use.
type Paper struct {
	log    *slog.Logger
	logged atomic.Bool
}

func NewPaper(log *slog.Logger) *Paper {
	if log == nil {
		log = slog.Default()
	}
	return &Paper{log: log}
}

// Admit runs Check and, on a drawdown halt, logs MAX_DRAWDOWN once per book.
func (p *Paper) Admit(in Intent, snap Snapshot) Decision {
	d := Check(in, snap)
	if d.Reason == ReasonMaxDrawdown {
		p.NoteHalt(snap)
	}
	return d
}

// NoteHalt logs MAX_DRAWDOWN a single time when the peak-to-trough halt trips.
func (p *Paper) NoteHalt(snap Snapshot) {
	if p == nil || !Halted(snap) {
		return
	}
	if p.logged.CompareAndSwap(false, true) {
		p.log.Info(ReasonMaxDrawdown,
			"equity", snap.Equity,
			"peak", snap.Peak,
			"drawdown", Drawdown(snap),
		)
	}
}

// Reset clears the once-log so a new campaign can halt again.
func (p *Paper) Reset() {
	p.logged.Store(false)
}

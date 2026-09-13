package builtin

type phase uint8

const (
	idle phase = iota
	surge
	rally
	pullback
	complete
	invalid
)

type trackerConfig struct {
	MaxPullbackPct  float64
	MaxPullbackBars int
	SurgeGap        int
	GradualDays     int
	RequireNearLow  bool
}

// trackerInput contains only values confirmed at the current bar. The tracker
// deliberately has no access to a Bar slice or a feature series.
type trackerInput struct {
	Index   int
	Close   float64
	Surge   bool
	Gradual bool
	NearLow bool
}

type trackerOutput struct {
	Signal bool
	Phase  phase
}

// pullbackTracker recognizes a completed surge followed by a bounded pullback
// while retaining only the state produced by earlier bars.
type pullbackTracker struct {
	config          trackerConfig
	phase           phase
	surgeIndex      int
	lastSurgeIndex  int
	peakIndex       int
	peakClose       float64
	gradualCount    int
	gradualEligible bool
}

func newPullbackTracker(config trackerConfig) *pullbackTracker {
	return &pullbackTracker{config: config, surgeIndex: -1, lastSurgeIndex: -1, peakIndex: -1}
}

func (p *pullbackTracker) Advance(input trackerInput) trackerOutput {
	if input.Close <= 0 || input.Index < 0 {
		p.phase = invalid
		return p.output(false)
	}
	// A terminal window belongs only to its own candidate. Reset and evaluate
	// this bar once as idle input so a later independent surge is not lost.
	if p.phase == idle {
		p.advanceIdle(input)
		return p.output(false)
	}
	if p.phase == invalid || p.phase == complete {
		p.reset()
		p.advanceIdle(input)
		return p.output(false)
	}

	if input.Surge {
		if p.config.SurgeGap > 0 && input.Index-p.lastSurgeIndex <= p.config.SurgeGap {
			p.lastSurgeIndex = input.Index
			if input.Close >= p.peakClose {
				p.peakIndex, p.peakClose = input.Index, input.Close
			}
			// A follow-on qualified surge extends the rally, even if it is below
			// the old peak; that same bar is never a pullback signal.
			p.phase = rally
			return p.output(false)
		}
		// Daily B1 does not merge separate surge days (SurgeGap is zero); a
		// new surge replaces the unfinished candidate. Bottom-surge candidates
		// outside their configured gap also restart only when idle permits it.
		p.reset()
		p.advanceIdle(input)
		return p.output(false)
	}
	if input.Close >= p.peakClose {
		p.peakIndex, p.peakClose, p.phase = input.Index, input.Close, rally
		return p.output(false)
	}

	pullbackBars := input.Index - p.peakIndex
	pullbackPct := (p.peakClose - input.Close) / p.peakClose * 100
	if pullbackBars > p.config.MaxPullbackBars || pullbackPct > p.config.MaxPullbackPct {
		p.phase = invalid
		return p.output(false)
	}
	p.phase = pullback
	return p.output(true)
}

func (p *pullbackTracker) advanceIdle(input trackerInput) {
	if input.Surge && (!p.config.RequireNearLow || input.NearLow) {
		p.beginSurge(input)
		return
	}
	if p.config.GradualDays <= 0 || !input.Gradual {
		p.gradualCount, p.gradualEligible = 0, false
		return
	}
	if p.gradualCount == 0 {
		p.gradualEligible = !p.config.RequireNearLow || input.NearLow
	}
	p.gradualCount++
	if p.gradualCount >= p.config.GradualDays && p.gradualEligible {
		p.beginSurge(input)
		// The final gradual bar is itself a confirmed part of the rally.
		p.phase = rally
	}
}

func (p *pullbackTracker) reset() {
	p.phase = idle
	p.surgeIndex, p.lastSurgeIndex, p.peakIndex = -1, -1, -1
	p.peakClose = 0
	p.gradualCount, p.gradualEligible = 0, false
}

func (p *pullbackTracker) beginSurge(input trackerInput) {
	p.phase = surge
	p.surgeIndex, p.lastSurgeIndex = input.Index, input.Index
	p.peakIndex, p.peakClose = input.Index, input.Close
	p.gradualCount, p.gradualEligible = 0, false
}

func (p *pullbackTracker) output(signal bool) trackerOutput {
	return trackerOutput{Signal: signal, Phase: p.phase}
}

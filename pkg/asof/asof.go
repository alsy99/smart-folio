package asof

import (
	"time"

	commonv1 "aperture/gen/common/v1"
)

// KnownAt is true when published is a real timestamp at or before the bar.
// A missing timestamp is not trusted for fills — reject it.
func KnownAt(published, bar time.Time) bool {
	if published.IsZero() || bar.IsZero() {
		return false
	}
	return !published.After(bar)
}

func ms(unixMs int64) time.Time {
	if unixMs <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(unixMs)
}

// News keeps headlines with published_at <= bar_time.
func News(items []*commonv1.NewsItem, bar time.Time) []*commonv1.NewsItem {
	out := make([]*commonv1.NewsItem, 0, len(items))
	for _, n := range items {
		if n == nil {
			continue
		}
		if KnownAt(ms(n.PublishedAtUnixMs), bar) {
			out = append(out, n)
		}
	}
	return out
}

// ReportAsOf is the latest source timestamp. The investigation did not exist
// as a complete object until every contributing headline had printed.
func ReportAsOf(r *commonv1.InvestigationReport) time.Time {
	if r == nil {
		return time.Time{}
	}
	var latest time.Time
	for _, src := range r.Sources {
		if src == nil {
			continue
		}
		t := ms(src.PublishedAtUnixMs)
		if t.IsZero() {
			return time.Time{}
		}
		if t.After(latest) {
			latest = t
		}
	}
	if latest.IsZero() {
		latest = ms(r.AnalyzedAtUnixMs)
	}
	return latest
}

// ReportKnown is false if any source printed after the bar (look-ahead).
func ReportKnown(r *commonv1.InvestigationReport, bar time.Time) bool {
	if r == nil {
		return false
	}
	if len(r.Sources) == 0 {
		return KnownAt(ms(r.AnalyzedAtUnixMs), bar)
	}
	for _, src := range r.Sources {
		if src == nil || !KnownAt(ms(src.PublishedAtUnixMs), bar) {
			return false
		}
	}
	return true
}

// Reports drops investigations that used any headline after the bar.
func Reports(in []*commonv1.InvestigationReport, bar time.Time) []*commonv1.InvestigationReport {
	out := make([]*commonv1.InvestigationReport, 0, len(in))
	for _, r := range in {
		if ReportKnown(r, bar) {
			out = append(out, r)
		}
	}
	return out
}

// Scores rebuilds per-name sentiment from as-of reports only.
func Scores(reports []*commonv1.InvestigationReport) map[string]float64 {
	out := map[string]float64{}
	conf := map[string]float64{}
	n := map[string]int{}
	for _, r := range reports {
		if r == nil {
			continue
		}
		for _, sym := range r.Symbols {
			out[sym] += r.Score * r.Confidence
			conf[sym] += r.Confidence
			n[sym]++
		}
	}
	for sym := range out {
		if n[sym] > 0 && conf[sym] > 0 {
			out[sym] = out[sym] / float64(n[sym])
		}
	}
	return out
}

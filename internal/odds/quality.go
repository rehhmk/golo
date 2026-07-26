package odds

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// ProviderObservation is a non-outcome-bearing audit of one provider poll.
// It deliberately records rejected rows so a clean-looking cache cannot hide
// parser or upstream quality regressions.
type ProviderObservation struct {
	Provider          string    `json:"provider"`
	ObservedAt        time.Time `json:"observedAt"`
	RawEvents         int       `json:"rawEvents"`
	AcceptedEvents    int       `json:"acceptedEvents"`
	RawQuotes         int       `json:"rawQuotes"`
	AcceptedQuotes    int       `json:"acceptedQuotes"`
	DuplicateEvents   int       `json:"duplicateEvents"`
	EmptyMarkets      int       `json:"emptyMarkets"`
	InvalidPrices     int       `json:"invalidPrices"`
	MissingTimestamps int       `json:"missingTimestamps"`
	StaleQuotes       int       `json:"staleQuotes"`
	MissingDeepLinks  int       `json:"missingDeepLinks"`
	Error             string    `json:"error,omitempty"`
}

// ProviderComparison describes only overlapping, independently observed
// events. A low overlap is visible instead of treating absence as agreement.
type ProviderComparison struct {
	ObservedAt          time.Time `json:"observedAt"`
	PrimaryProvider     string    `json:"primaryProvider"`
	ShadowProvider      string    `json:"shadowProvider"`
	PrimaryEvents       int       `json:"primaryEvents"`
	ShadowEvents        int       `json:"shadowEvents"`
	MatchedEvents       int       `json:"matchedEvents"`
	ComparableQuotes    int       `json:"comparableQuotes"`
	ScoreMismatches     int       `json:"scoreMismatches"`
	MeanAbsoluteOverGap float64   `json:"meanAbsoluteOverGap"`
}

// ValidateSnapshot rejects duplicates, incomplete pairs and impossible prices
// before an upstream payload reaches a cache.
func ValidateSnapshot(provider string, events []Event, now time.Time, maximumAge time.Duration) ([]Event, ProviderObservation) {
	observation := ProviderObservation{Provider: provider, ObservedAt: now, RawEvents: len(events)}
	seen := map[string]struct{}{}
	clean := make([]Event, 0, len(events))
	for _, event := range events {
		key := event.ID
		if key == "" {
			key = eventKey(event)
		}
		if _, exists := seen[key]; exists {
			observation.DuplicateEvents++
			continue
		}
		seen[key] = struct{}{}
		filtered := event
		filtered.Provider = provider
		filtered.Quotes = nil
		observation.RawQuotes += len(event.Quotes)
		if len(event.Quotes) == 0 {
			observation.EmptyMarkets++
		}
		for _, quote := range event.Quotes {
			quote.Provider = provider
			if quote.Over <= 1 || quote.Under <= 1 ||
				math.IsNaN(quote.Over) || math.IsNaN(quote.Under) ||
				math.IsInf(quote.Over, 0) || math.IsInf(quote.Under, 0) {
				observation.InvalidPrices++
				continue
			}
			if quote.UpdatedAt.IsZero() {
				observation.MissingTimestamps++
				continue
			}
			if maximumAge > 0 && (now.Before(quote.UpdatedAt) || now.Sub(quote.UpdatedAt) > maximumAge) {
				observation.StaleQuotes++
				continue
			}
			if quote.DeepLink == "" {
				observation.MissingDeepLinks++
			}
			filtered.Quotes = append(filtered.Quotes, quote)
		}
		if len(filtered.Quotes) == 0 {
			continue
		}
		sort.Slice(filtered.Quotes, func(i, j int) bool {
			if filtered.Quotes[i].Line == filtered.Quotes[j].Line {
				return filtered.Quotes[i].Bookmaker < filtered.Quotes[j].Bookmaker
			}
			return filtered.Quotes[i].Line < filtered.Quotes[j].Line
		})
		observation.AcceptedQuotes += len(filtered.Quotes)
		clean = append(clean, filtered)
	}
	observation.AcceptedEvents = len(clean)
	return clean, observation
}

func CompareSnapshots(primaryProvider string, primary []Event, shadowProvider string, shadow []Event, now time.Time) ProviderComparison {
	report := ProviderComparison{
		ObservedAt: now, PrimaryProvider: primaryProvider, ShadowProvider: shadowProvider,
		PrimaryEvents: len(primary), ShadowEvents: len(shadow),
	}
	shadowByKey := map[string][]Event{}
	for _, event := range shadow {
		shadowByKey[eventKey(event)] = append(shadowByKey[eventKey(event)], event)
	}
	totalGap := 0.0
	for _, left := range primary {
		candidates := shadowByKey[eventKey(left)]
		var right *Event
		for i := range candidates {
			if kickoffClose(left.ScheduledAt, candidates[i].ScheduledAt) {
				candidate := candidates[i]
				right = &candidate
				break
			}
		}
		if right == nil {
			continue
		}
		report.MatchedEvents++
		if left.HomeScore != right.HomeScore || left.AwayScore != right.AwayScore {
			report.ScoreMismatches++
			continue
		}
		for _, lq := range left.Quotes {
			for _, rq := range right.Quotes {
				if lq.Market == rq.Market && math.Abs(lq.Line-rq.Line) < .001 {
					report.ComparableQuotes++
					totalGap += math.Abs(lq.Over - rq.Over)
					break
				}
			}
		}
	}
	if report.ComparableQuotes > 0 {
		report.MeanAbsoluteOverGap = totalGap / float64(report.ComparableQuotes)
	}
	return report
}

func eventKey(event Event) string {
	return fmt.Sprintf("%s:%s", normalizeName(event.Home), normalizeName(event.Away))
}

func kickoffClose(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return true
	}
	delta := a.Sub(b)
	return delta >= -10*time.Minute && delta <= 10*time.Minute
}

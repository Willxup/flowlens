package query

import (
	"errors"
	"time"

	"github.com/Willxup/flowlens/internal/rollup"
)

// ErrRangeSelection means a public historical range selection is invalid.
var ErrRangeSelection = errors.New("invalid FlowLens historical range selection")

// RangeKind is one server-planned historical range.
type RangeKind string

const (
	RangeToday      RangeKind = "today"
	RangeYesterday  RangeKind = "yesterday"
	RangeSevenDays  RangeKind = "7d"
	RangeThirtyDays RangeKind = "30d"
	RangeNinetyDays RangeKind = "90d"
	RangeYear       RangeKind = "year"
	RangeLifetime   RangeKind = "lifetime"
	RangeCustom     RangeKind = "custom"
)

// RangeSelection carries user intent without client-generated Unix times.
type RangeSelection struct {
	Kind     RangeKind
	FromDate string
	ToDate   string
}

// ResolveRange converts public range intent with the server clock and configured timezone.
func (s *Service) ResolveRange(selection RangeSelection) (rollup.Range, error) {
	now := s.now()
	if now.Unix() <= 86_400 {
		return rollup.Range{}, ErrRangeSelection
	}
	localNow := now.In(s.location)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, s.location)
	if selection.Kind != RangeCustom && (selection.FromDate != "" || selection.ToDate != "") {
		return rollup.Range{}, ErrRangeSelection
	}

	var result rollup.Range
	switch selection.Kind {
	case RangeToday:
		result = rollup.Range{From: today.Unix(), To: now.Unix()}
	case RangeYesterday:
		result = rollup.Range{From: today.AddDate(0, 0, -1).Unix(), To: today.Unix()}
	case RangeSevenDays:
		result = rollup.Range{From: now.Add(-7 * 24 * time.Hour).Unix(), To: now.Unix()}
	case RangeThirtyDays:
		result = rollup.Range{From: now.Add(-30 * 24 * time.Hour).Unix(), To: now.Unix()}
	case RangeNinetyDays:
		result = rollup.Range{From: now.Add(-90 * 24 * time.Hour).Unix(), To: now.Unix()}
	case RangeYear:
		result = rollup.Range{
			From: time.Date(localNow.Year(), time.January, 1, 0, 0, 0, 0, s.location).Unix(),
			To:   now.Unix(),
		}
	case RangeLifetime:
		result = rollup.Range{From: 86_400, To: now.Unix()}
	case RangeCustom:
		from, err := time.ParseInLocation(time.DateOnly, selection.FromDate, s.location)
		if err != nil || from.Format(time.DateOnly) != selection.FromDate {
			return rollup.Range{}, ErrRangeSelection
		}
		toDate, err := time.ParseInLocation(time.DateOnly, selection.ToDate, s.location)
		if err != nil || toDate.Format(time.DateOnly) != selection.ToDate ||
			toDate.Before(from) || toDate.After(today) {
			return rollup.Range{}, ErrRangeSelection
		}
		to := toDate.AddDate(0, 0, 1).Unix()
		if toDate.Equal(today) {
			to = now.Unix()
		}
		result = rollup.Range{From: from.Unix(), To: to}
	default:
		return rollup.Range{}, ErrRangeSelection
	}
	if result.From <= 0 || result.To <= result.From || result.To > now.Unix() {
		return rollup.Range{}, ErrRangeSelection
	}
	return result, nil
}

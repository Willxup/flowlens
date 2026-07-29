package httpapi

import (
	"net/url"

	"github.com/Willxup/flowlens/internal/query"
)

func strictRangeSelection(values url.Values, extras map[string]bool) (query.RangeSelection, bool) {
	if len(values["range"]) != 1 {
		return query.RangeSelection{}, false
	}
	for key, entries := range values {
		if len(entries) != 1 || (key != "range" && key != "from" && key != "to" && !extras[key]) {
			return query.RangeSelection{}, false
		}
	}
	kind := query.RangeKind(values.Get("range"))
	selection := query.RangeSelection{Kind: kind}
	switch kind {
	case query.RangeCustom:
		if len(values["from"]) != 1 || len(values["to"]) != 1 {
			return query.RangeSelection{}, false
		}
		selection.FromDate = values.Get("from")
		selection.ToDate = values.Get("to")
	case query.RangeToday, query.RangeYesterday, query.RangeSevenDays, query.RangeThirtyDays,
		query.RangeNinetyDays, query.RangeYear, query.RangeLifetime:
		if len(values["from"]) != 0 || len(values["to"]) != 0 {
			return query.RangeSelection{}, false
		}
	default:
		return query.RangeSelection{}, false
	}
	return selection, true
}

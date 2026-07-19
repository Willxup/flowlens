package query

import (
	"context"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

// Labels returns all aliases in stable storage order.
func (service *Service) Labels(ctx context.Context) ([]Label, error) {
	values, err := service.store.Labels(ctx)
	if err != nil {
		return nil, ErrQuery
	}
	result := make([]Label, 0, len(values))
	for _, value := range values {
		result = append(result, publicLabel(value))
	}
	return result, nil
}

// CreateLabel validates and writes one canonical alias.
func (service *Service) CreateLabel(ctx context.Context, input CreateLabel) (Label, error) {
	match, ok := canonicalLabelKey(input.LabelType, input.MatchValue)
	display, displayOK := normalizedDisplayName(input.DisplayName)
	if !ok || !displayOK {
		return Label{}, storage.ErrInvalidLabel
	}
	now := service.now().Unix()
	value, err := service.store.CreateLabel(ctx, storage.ServiceLabel{
		LabelType: input.LabelType, MatchValue: match, DisplayName: display, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return Label{}, err
	}
	return publicLabel(value), nil
}

// UpdateLabel changes only one display name.
func (service *Service) UpdateLabel(ctx context.Context, id int64, displayName string) (Label, error) {
	display, ok := normalizedDisplayName(displayName)
	if id <= 0 || !ok {
		return Label{}, storage.ErrInvalidLabel
	}
	value, err := service.store.UpdateLabel(ctx, id, display, service.now().Unix())
	if err != nil {
		return Label{}, err
	}
	return publicLabel(value), nil
}

// DeleteLabel removes one display alias without touching flow history.
func (service *Service) DeleteLabel(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, storage.ErrInvalidLabel
	}
	return service.store.DeleteLabel(ctx, id)
}

// LabelCandidates discovers bounded host and endpoint keys from the latest
// rolling 30-day non-overlapping flow plan.
func (service *Service) LabelCandidates(ctx context.Context) ([]LabelCandidate, error) {
	now := service.now()
	rangeValue := rollup.Range{From: now.Add(-30 * 24 * time.Hour).Unix(), To: now.Unix()}
	segments, _, err := rollup.PlanSeries(rangeValue, now, service.retention, service.location)
	if err != nil {
		return nil, ErrQuery
	}
	segments[0].From = rangeValue.From
	segments[len(segments)-1].To = rangeValue.To
	exactSegments := segments[:0]
	for _, segment := range segments {
		if segment.From < segment.To {
			exactSegments = append(exactSegments, segment)
		}
	}
	if len(exactSegments) == 0 {
		return nil, ErrQuery
	}
	points, err := service.store.FlowSeries(ctx, exactSegments)
	if err != nil {
		return nil, ErrQuery
	}
	type aggregate struct {
		labelType string
		match     string
		upload    int64
		download  int64
	}
	aggregates := make(map[string]*aggregate)
	for _, point := range points {
		if point.Dimension.ClassificationCode != 1 {
			continue
		}
		address, valid := dimensionAddress(point.Dimension.DestinationFamily, point.Dimension.DestinationIP)
		if !valid {
			continue
		}
		keys := [][2]string{{"host", address.String()}}
		if endpoint := attribution.EndpointValue(point.Dimension); endpoint != "" {
			keys = append(keys, [2]string{"endpoint", endpoint})
		}
		for _, key := range keys {
			mapKey := key[0] + "\x00" + key[1]
			value := aggregates[mapKey]
			if value == nil {
				value = &aggregate{labelType: key[0], match: key[1]}
				aggregates[mapKey] = value
			}
			if !safeQueryAdd(&value.upload, point.UploadBytes) || !safeQueryAdd(&value.download, point.DownloadBytes) {
				return nil, ErrQuery
			}
		}
	}
	aliases, err := service.labelResolver(ctx)
	if err != nil {
		return nil, ErrQuery
	}
	result := make([]LabelCandidate, 0, len(aggregates))
	for _, value := range aggregates {
		display := aliases.display(value.labelType, value.match)
		result = append(result, LabelCandidate{
			LabelType: value.labelType, MatchValue: value.match, DisplayName: display,
			UploadBytes: value.upload, DownloadBytes: value.download,
		})
	}
	sort.Slice(result, func(left, right int) bool {
		leftTotal := uint64(result[left].UploadBytes) + uint64(result[left].DownloadBytes)
		rightTotal := uint64(result[right].UploadBytes) + uint64(result[right].DownloadBytes)
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		if result[left].MatchValue != result[right].MatchValue {
			return result[left].MatchValue < result[right].MatchValue
		}
		return result[left].LabelType < result[right].LabelType
	})
	if len(result) > 100 {
		result = result[:100]
	}
	return result, nil
}

type resolver struct {
	hosts     map[string]string
	endpoints map[string]string
}

func (service *Service) labelResolver(ctx context.Context) (resolver, error) {
	values, err := service.store.Labels(ctx)
	if err != nil {
		return resolver{}, err
	}
	result := resolver{hosts: make(map[string]string), endpoints: make(map[string]string)}
	for _, value := range values {
		if value.LabelType == "host" {
			result.hosts[value.MatchValue] = value.DisplayName
		} else if value.LabelType == "endpoint" {
			result.endpoints[value.MatchValue] = value.DisplayName
		}
	}
	return result, nil
}

func (value resolver) display(labelType, raw string) string {
	if labelType == "host" {
		if display := value.hosts[raw]; display != "" {
			return display
		}
		return raw
	}
	if display := value.endpoints[raw]; display != "" {
		return display
	}
	if endpoint, err := netip.ParseAddrPort(raw); err == nil {
		if display := value.hosts[endpoint.Addr().Unmap().String()]; display != "" {
			return display + ":" + strconv.Itoa(int(endpoint.Port()))
		}
	}
	return raw
}

func canonicalLabelKey(labelType, raw string) (string, bool) {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return "", false
	}
	switch labelType {
	case "host":
		address, err := netip.ParseAddr(raw)
		if err != nil {
			return "", false
		}
		canonical := address.Unmap().String()
		return canonical, raw == canonical
	case "endpoint":
		endpoint, err := netip.ParseAddrPort(raw)
		if err != nil || endpoint.Port() == 0 {
			return "", false
		}
		canonical := netip.AddrPortFrom(endpoint.Addr().Unmap(), endpoint.Port()).String()
		return canonical, raw == canonical
	default:
		return "", false
	}
}

func normalizedDisplayName(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 64 {
		return "", false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", false
		}
	}
	return value, true
}

func publicLabel(value storage.ServiceLabel) Label {
	return Label{
		ID: value.ID, LabelType: value.LabelType, MatchValue: value.MatchValue,
		DisplayName: value.DisplayName, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

package query

import (
	"context"
	"math"
	"net/netip"
	"sort"
	"strconv"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/rollup"
	"github.com/Willxup/flowlens/internal/storage"
)

type projectedFlow struct {
	item       BreakdownItem
	validValue bool
	networkSet bool
}

// Breakdown returns one bounded, conserving approximate dimensional view.
func (service *Service) Breakdown(ctx context.Context, rangeValue rollup.Range, by BreakdownBy) (Breakdown, error) {
	if !validBreakdownBy(by) {
		return Breakdown{}, ErrQuery
	}
	segments, approximate, err := rollup.PlanSeries(rangeValue, service.now(), service.retention, service.location)
	if err != nil {
		return Breakdown{}, ErrQuery
	}
	globalPoints, flowPoints, err := service.store.BreakdownSeries(ctx, segments)
	if err != nil {
		return Breakdown{}, ErrQuery
	}
	result := Breakdown{By: by, BoundaryApproximate: approximate}
	for _, point := range globalPoints {
		if !addByteTotals(&result.Global, point.UploadBytes, point.DownloadBytes) {
			return Breakdown{}, ErrQuery
		}
	}
	projected := make(map[string]*projectedFlow)
	hasValidValue := false
	for _, point := range flowPoints {
		switch point.Dimension.ClassificationCode {
		case 2:
			if !addByteTotals(&result.Other, point.UploadBytes, point.DownloadBytes) {
				return Breakdown{}, ErrQuery
			}
		case 3:
			if !addByteTotals(&result.Unattributed, point.UploadBytes, point.DownloadBytes) {
				return Breakdown{}, ErrQuery
			}
		case 1:
			raw, network, valid := projectDimension(point.Dimension, by)
			hasValidValue = hasValidValue || valid
			entry := projected[raw]
			if entry == nil {
				entry = &projectedFlow{item: BreakdownItem{
					Key: raw, RawValue: raw, DisplayName: raw, NetworkCode: network,
				}, validValue: valid, networkSet: true}
				projected[raw] = entry
			} else {
				entry.validValue = entry.validValue || valid
				if entry.networkSet && entry.item.NetworkCode != network {
					entry.item.NetworkCode = 0
				}
			}
			if !safeQueryAdd(&entry.item.UploadBytes, point.UploadBytes) || !safeQueryAdd(&entry.item.DownloadBytes, point.DownloadBytes) {
				return Breakdown{}, ErrQuery
			}
		default:
			return Breakdown{}, ErrQuery
		}
	}
	result.Available = service.projectionAvailable(by, hasValidValue)
	items := make([]BreakdownItem, 0, len(projected))
	if result.Available {
		for _, value := range projected {
			items = append(items, value.item)
		}
		sortBreakdownItems(items)
		if len(items) > service.retention.TopK {
			for _, item := range items[service.retention.TopK:] {
				if !addByteTotals(&result.Other, item.UploadBytes, item.DownloadBytes) {
					return Breakdown{}, ErrQuery
				}
			}
			items = items[:service.retention.TopK]
		}
	} else {
		for _, value := range projected {
			if !addByteTotals(&result.Other, value.item.UploadBytes, value.item.DownloadBytes) {
				return Breakdown{}, ErrQuery
			}
		}
	}
	result.Items = items
	if by == ByTarget || by == ByEndpoint {
		aliases, err := service.labelResolver(ctx)
		if err != nil {
			return Breakdown{}, ErrQuery
		}
		labelType := "host"
		if by == ByEndpoint {
			labelType = "endpoint"
		}
		for index := range result.Items {
			result.Items[index].DisplayName = aliases.display(labelType, result.Items[index].RawValue)
		}
	}
	var represented storage.ByteTotals
	for _, item := range items {
		if !addByteTotals(&represented, item.UploadBytes, item.DownloadBytes) {
			return Breakdown{}, ErrQuery
		}
	}
	if !addByteTotals(&represented, result.Other.Upload, result.Other.Download) ||
		!addByteTotals(&represented, result.Unattributed.Upload, result.Unattributed.Download) ||
		represented != result.Global {
		return Breakdown{}, ErrQuery
	}
	globalCombined := float64(result.Global.Upload) + float64(result.Global.Download)
	if globalCombined == 0 {
		result.NoTraffic = true
		return result, nil
	}
	coveredCombined := float64(result.Global.Upload-result.Unattributed.Upload) +
		float64(result.Global.Download-result.Unattributed.Download)
	coverage := coveredCombined / globalCombined
	result.ConnectionCoverage = &coverage
	if coveredCombined > 0 {
		attributedCombined := float64(represented.Upload-result.Other.Upload-result.Unattributed.Upload) +
			float64(represented.Download-result.Other.Download-result.Unattributed.Download)
		retention := attributedCombined / coveredCombined
		result.DimensionRetention = &retention
	}
	return result, nil
}

// LiveTargets maps the immutable attribution snapshot to a detached query model.
func (service *Service) LiveTargets(ctx context.Context) (LiveTargets, error) {
	if err := ctx.Err(); err != nil {
		return LiveTargets{}, ErrQuery
	}
	snapshot := service.live.Snapshot()
	aliases, err := service.labelResolver(ctx)
	if err != nil {
		return LiveTargets{}, ErrQuery
	}
	result := LiveTargets{
		ObservedAt: snapshot.ObservedAt, IntervalMillis: snapshot.IntervalMillis,
		ActiveConnections: snapshot.ActiveConnections,
	}
	if snapshot.ConnectionCoverage != nil {
		coverage := *snapshot.ConnectionCoverage
		result.ConnectionCoverage = &coverage
	}
	result.Targets = make([]LiveTarget, 0, len(snapshot.Targets))
	for _, target := range snapshot.Targets {
		result.Targets = append(result.Targets, LiveTarget{
			RawEndpoint: target.RawEndpoint, DisplayName: aliases.display("endpoint", target.RawEndpoint),
			NetworkCode: target.NetworkCode, Host: target.Host,
			UploadBytesPerSecond:   target.UploadBytesPerSecond,
			DownloadBytesPerSecond: target.DownloadBytesPerSecond,
		})
	}
	return result, nil
}

// RuntimeSessions returns the fixed newest 100 public-safe session records.
func (service *Service) RuntimeSessions(ctx context.Context) ([]RuntimeSessionRecord, error) {
	sessions, err := service.store.RuntimeSessions(ctx, 100)
	if err != nil {
		return nil, ErrQuery
	}
	result := make([]RuntimeSessionRecord, 0, len(sessions))
	for _, session := range sessions {
		result = append(result, RuntimeSessionRecord{
			StartedAt: session.StartedAt, EndedAt: copyInt64(session.EndedAt),
			StartReason: session.StartReason, EndReason: copyString(session.EndReason),
			LastSeenAt: session.LastSeenAt, SingBoxVersion: session.SingBoxVersion,
			DataGapBeforeSeconds: session.DataGapBeforeSeconds,
		})
	}
	return result, nil
}

func (service *Service) projectionAvailable(by BreakdownBy, historical bool) bool {
	if by == BySource && service.privacy == attribution.SourceDisabled {
		return false
	}
	capabilities := service.live.Capabilities()
	return historical || requiredCapability(capabilities, by)
}

func requiredCapability(capabilities clashapi.DimensionCapabilities, by BreakdownBy) bool {
	switch by {
	case ByTarget:
		return capabilities.DestinationIP
	case ByEndpoint:
		return capabilities.DestinationIP && capabilities.DestinationPort
	case ByPort:
		return capabilities.DestinationPort
	case ByNetwork:
		return capabilities.Network
	case BySource:
		return capabilities.SourceIP
	case ByDomain:
		return capabilities.Host
	default:
		return false
	}
}

func projectDimension(dimension storage.FlowDimension, by BreakdownBy) (string, int64, bool) {
	network := dimension.NetworkCode
	switch by {
	case ByTarget:
		address, valid := dimensionAddress(dimension.DestinationFamily, dimension.DestinationIP)
		if !valid {
			return "unknown", network, false
		}
		return address.String(), network, true
	case ByEndpoint:
		value := attribution.EndpointValue(dimension)
		if value == "" {
			return "unknown", network, false
		}
		return value, network, true
	case ByPort:
		if dimension.DestinationPort < 1 || dimension.DestinationPort > 65535 {
			return "unknown", network, false
		}
		return strconv.FormatInt(dimension.DestinationPort, 10), network, true
	case ByNetwork:
		switch network {
		case 1:
			return "tcp", 1, true
		case 2:
			return "udp", 2, true
		default:
			return "unknown", 0, false
		}
	case BySource:
		address, valid := dimensionAddress(dimension.SourceFamily, dimension.SourceNetwork)
		if !valid || dimension.SourcePrefixLen < 0 || dimension.SourcePrefixLen > int64(address.BitLen()) {
			return "unknown", network, false
		}
		return netip.PrefixFrom(address, int(dimension.SourcePrefixLen)).Masked().String(), network, true
	case ByDomain:
		if dimension.Host == "" {
			return "unknown", network, false
		}
		return dimension.Host, network, true
	default:
		return "unknown", network, false
	}
}

func dimensionAddress(family int64, raw []byte) (netip.Addr, bool) {
	address, ok := netip.AddrFromSlice(raw)
	if !ok {
		return netip.Addr{}, false
	}
	address = address.Unmap()
	if (family == 4 && address.Is4()) || (family == 6 && address.Is6() && !address.Is4()) {
		return address, true
	}
	return netip.Addr{}, false
}

func validBreakdownBy(by BreakdownBy) bool {
	return by == ByTarget || by == ByEndpoint || by == ByPort || by == ByNetwork || by == BySource || by == ByDomain
}

func sortBreakdownItems(items []BreakdownItem) {
	sort.Slice(items, func(left, right int) bool {
		leftTotal := uint64(items[left].UploadBytes) + uint64(items[left].DownloadBytes)
		rightTotal := uint64(items[right].UploadBytes) + uint64(items[right].DownloadBytes)
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		return items[left].RawValue < items[right].RawValue
	})
}

func addByteTotals(target *storage.ByteTotals, upload, download int64) bool {
	return safeQueryAdd(&target.Upload, upload) && safeQueryAdd(&target.Download, download)
}

func safeQueryAdd(target *int64, value int64) bool {
	if value < 0 || *target > math.MaxInt64-value {
		return false
	}
	*target += value
	return true
}

func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

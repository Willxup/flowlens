package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"strings"
)

// ErrInvalidBatch means a batch violates the durable storage contract.
var ErrInvalidBatch = errors.New("invalid FlowLens storage batch")

func validateBatch(batch Batch) error {
	if !validBoundedText(batch.BatchID, 128) || !validBoundedText(batch.NewState.RuntimeSessionID, 128) {
		return ErrInvalidBatch
	}
	if !validBoundedText(batch.NewState.BucketTimezone, 128) || batch.NewState.LastSampleAt <= 0 {
		return ErrInvalidBatch
	}
	if err := validateByteTotals(batch.NewState.LastTotals); err != nil {
		return err
	}
	if batch.ExpectedOldTotals != nil {
		if err := validateByteTotals(*batch.ExpectedOldTotals); err != nil {
			return err
		}
	}
	if err := validateTrafficRollup(batch.Global); err != nil {
		return err
	}
	if batch.NewState.LastSampleAt < batch.Global.BucketStart || batch.NewState.LastSampleAt >= batch.Global.BucketEnd {
		return ErrInvalidBatch
	}
	if err := validateRuntimeTransition(batch); err != nil {
		return err
	}
	if err := validateFlows(batch.Global, batch.Flows); err != nil {
		return err
	}
	if len(batch.QualityEvents) > 128 {
		return ErrInvalidBatch
	}
	for _, event := range batch.QualityEvents {
		if !validBoundedText(event.Code, 64) || event.StartedAt <= 0 || event.Flags < 0 || len(event.Detail) > 4096 {
			return ErrInvalidBatch
		}
		if event.EndedAt != nil && *event.EndedAt < event.StartedAt {
			return ErrInvalidBatch
		}
	}
	return nil
}

func validateByteTotals(totals ByteTotals) error {
	if totals.Upload < 0 || totals.Download < 0 {
		return ErrInvalidBatch
	}
	return nil
}

func validateTrafficRollup(rollup TrafficRollup) error {
	if rollup.ResolutionSec != 10 || rollup.BucketStart <= 0 ||
		rollup.BucketEnd <= rollup.BucketStart || rollup.BucketEnd-rollup.BucketStart != 10 {
		return ErrInvalidBatch
	}
	values := []int64{
		rollup.UploadBytes,
		rollup.DownloadBytes,
		rollup.RecoveredUploadBytes,
		rollup.RecoveredDownloadBytes,
		rollup.SpeedUploadSampleSum,
		rollup.SpeedDownloadSampleSum,
		rollup.SpeedSampleCount,
		rollup.PeakUploadBytesPerSecond,
		rollup.PeakDownloadBytesPerSecond,
		rollup.CounterObservedSeconds,
		rollup.AttributionObservedSeconds,
		rollup.ActiveConnectionsSum,
		rollup.ActiveConnectionsSamples,
		rollup.ActiveConnectionsMax,
		rollup.MemoryBytesSum,
		rollup.MemorySamples,
		rollup.MemoryBytesMax,
		rollup.UnattributedUploadBytes,
		rollup.UnattributedDownloadBytes,
		rollup.ResetCount,
		rollup.QualityFlags,
	}
	for _, value := range values {
		if value < 0 {
			return ErrInvalidBatch
		}
	}
	if rollup.RecoveredUploadBytes > rollup.UploadBytes ||
		rollup.RecoveredDownloadBytes > rollup.DownloadBytes ||
		rollup.UnattributedUploadBytes > rollup.UploadBytes ||
		rollup.UnattributedDownloadBytes > rollup.DownloadBytes {
		return ErrInvalidBatch
	}
	if rollup.CounterObservedSeconds > 10 || rollup.AttributionObservedSeconds > 10 {
		return ErrInvalidBatch
	}
	for _, peakAt := range []*int64{rollup.PeakUploadAt, rollup.PeakDownloadAt} {
		if peakAt != nil && (*peakAt < rollup.BucketStart || *peakAt >= rollup.BucketEnd) {
			return ErrInvalidBatch
		}
	}
	return nil
}

func validateRuntimeTransition(batch Batch) error {
	if batch.NewRuntimeSession != nil {
		start := batch.NewRuntimeSession
		if !validBoundedText(start.ID, 128) || start.ID != batch.NewState.RuntimeSessionID ||
			start.StartedAt <= 0 || start.StartedAt > batch.NewState.LastSampleAt ||
			!validBoundedText(start.StartReason, 64) || !validBoundedText(start.SingBoxVersion, 256) ||
			start.DataGapBeforeSeconds < 0 {
			return ErrInvalidBatch
		}
		if start.HostBootID != nil && !validBoundedText(*start.HostBootID, 128) {
			return ErrInvalidBatch
		}
	}
	if batch.EndRuntimeSession != nil {
		end := batch.EndRuntimeSession
		if !validBoundedText(end.ID, 128) || end.EndedAt <= 0 || !validBoundedText(end.EndReason, 64) {
			return ErrInvalidBatch
		}
		if batch.NewRuntimeSession != nil &&
			(end.ID == batch.NewRuntimeSession.ID || end.EndedAt > batch.NewRuntimeSession.StartedAt) {
			return ErrInvalidBatch
		}
	}
	return nil
}

func validateFlows(global TrafficRollup, flows []FlowRollup) error {
	if len(flows) > 102 {
		return ErrInvalidBatch
	}
	keys := make(map[string]struct{}, len(flows))
	var uploadTotal, downloadTotal, unattributedUpload, unattributedDownload int64
	for _, flow := range flows {
		if flow.UploadBytes < 0 || flow.DownloadBytes < 0 || flow.FlowObservationCount < 0 {
			return ErrInvalidBatch
		}
		if err := validateDimension(flow.Dimension); err != nil {
			return err
		}
		key := dimensionKey(flow.Dimension)
		if _, exists := keys[key]; exists {
			return ErrInvalidBatch
		}
		keys[key] = struct{}{}
		if !safeAdd(&uploadTotal, flow.UploadBytes) || !safeAdd(&downloadTotal, flow.DownloadBytes) {
			return ErrInvalidBatch
		}
		if flow.Dimension.ClassificationCode == 3 {
			if !safeAdd(&unattributedUpload, flow.UploadBytes) || !safeAdd(&unattributedDownload, flow.DownloadBytes) {
				return ErrInvalidBatch
			}
		}
	}
	if uploadTotal != global.UploadBytes || downloadTotal != global.DownloadBytes ||
		unattributedUpload != global.UnattributedUploadBytes ||
		unattributedDownload != global.UnattributedDownloadBytes {
		return ErrInvalidBatch
	}
	return nil
}

func validateDimension(dimension FlowDimension) error {
	if dimension.SourceFamily != 0 && dimension.SourceFamily != 4 && dimension.SourceFamily != 6 {
		return ErrInvalidBatch
	}
	if (dimension.SourceFamily == 0 && (len(dimension.SourceNetwork) != 0 || dimension.SourcePrefixLen != 0)) ||
		(dimension.SourceFamily == 4 && (len(dimension.SourceNetwork) != 4 || dimension.SourcePrefixLen < 0 || dimension.SourcePrefixLen > 32)) ||
		(dimension.SourceFamily == 6 && (len(dimension.SourceNetwork) != 16 || dimension.SourcePrefixLen < 0 || dimension.SourcePrefixLen > 128)) {
		return ErrInvalidBatch
	}
	if dimension.DestinationFamily != 0 && dimension.DestinationFamily != 4 && dimension.DestinationFamily != 6 {
		return ErrInvalidBatch
	}
	if (dimension.DestinationFamily == 0 && len(dimension.DestinationIP) != 0) ||
		(dimension.DestinationFamily == 4 && len(dimension.DestinationIP) != 4) ||
		(dimension.DestinationFamily == 6 && len(dimension.DestinationIP) != 16) {
		return ErrInvalidBatch
	}
	if dimension.DestinationPort < -1 || dimension.DestinationPort > 65535 ||
		len(dimension.Host) > 253 || dimension.NetworkCode < 0 || dimension.NetworkCode > 2 ||
		dimension.ClassificationCode < 1 || dimension.ClassificationCode > 3 {
		return ErrInvalidBatch
	}
	if dimension.ClassificationCode != 1 &&
		(dimension.SourceFamily != 0 || len(dimension.SourceNetwork) != 0 || dimension.SourcePrefixLen != 0 ||
			dimension.DestinationFamily != 0 || len(dimension.DestinationIP) != 0 || dimension.DestinationPort != -1 ||
			dimension.Host != "" || dimension.NetworkCode != 0) {
		return ErrInvalidBatch
	}
	return nil
}

func dimensionKey(dimension FlowDimension) string {
	var buffer bytes.Buffer
	writeInt64 := func(value int64) {
		_ = binary.Write(&buffer, binary.BigEndian, value)
	}
	writeBytes := func(value []byte) {
		writeInt64(int64(len(value)))
		_, _ = buffer.Write(value)
	}
	writeInt64(dimension.SourceFamily)
	writeBytes(dimension.SourceNetwork)
	writeInt64(dimension.SourcePrefixLen)
	writeInt64(dimension.DestinationFamily)
	writeBytes(dimension.DestinationIP)
	writeInt64(dimension.DestinationPort)
	writeBytes([]byte(dimension.Host))
	writeInt64(dimension.NetworkCode)
	writeInt64(dimension.ClassificationCode)
	return buffer.String()
}

func safeAdd(total *int64, value int64) bool {
	if value < 0 || *total > math.MaxInt64-value {
		return false
	}
	*total += value
	return true
}

func validBoundedText(value string, maximum int) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= maximum
}

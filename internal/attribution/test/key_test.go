package attribution_test

import (
	"sort"
	"testing"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/storage"
)

func TestDimensionKeyIsStableAndCollisionSafe(t *testing.T) {
	base := storage.FlowDimension{
		SourceFamily: 4, SourceNetwork: []byte{192, 0, 2, 0}, SourcePrefixLen: 24,
		DestinationFamily: 4, DestinationIP: []byte{198, 51, 100, 7}, DestinationPort: 443,
		Host: "a|b", NetworkCode: 1, ClassificationCode: 1,
	}
	copyValue := base
	copyValue.SourceNetwork = append([]byte(nil), base.SourceNetwork...)
	copyValue.DestinationIP = append([]byte(nil), base.DestinationIP...)
	if attribution.DimensionKey(base) != attribution.DimensionKey(copyValue) {
		t.Fatal("equal dimensions have different keys")
	}
	changes := []storage.FlowDimension{
		{SourceFamily: 6, SourceNetwork: base.SourceNetwork, SourcePrefixLen: 24, DestinationFamily: 4, DestinationIP: base.DestinationIP, DestinationPort: 443, Host: "a|b", NetworkCode: 1, ClassificationCode: 1},
		{SourceFamily: 4, SourceNetwork: []byte{192, 0, 2, 1}, SourcePrefixLen: 24, DestinationFamily: 4, DestinationIP: base.DestinationIP, DestinationPort: 443, Host: "a|b", NetworkCode: 1, ClassificationCode: 1},
		{SourceFamily: 4, SourceNetwork: base.SourceNetwork, SourcePrefixLen: 32, DestinationFamily: 4, DestinationIP: base.DestinationIP, DestinationPort: 443, Host: "a|b", NetworkCode: 1, ClassificationCode: 1},
		{SourceFamily: 4, SourceNetwork: base.SourceNetwork, SourcePrefixLen: 24, DestinationFamily: 4, DestinationIP: []byte{198, 51, 100, 8}, DestinationPort: 443, Host: "a|b", NetworkCode: 1, ClassificationCode: 1},
		{SourceFamily: 4, SourceNetwork: base.SourceNetwork, SourcePrefixLen: 24, DestinationFamily: 4, DestinationIP: base.DestinationIP, DestinationPort: 80, Host: "a|b", NetworkCode: 1, ClassificationCode: 1},
		{SourceFamily: 4, SourceNetwork: base.SourceNetwork, SourcePrefixLen: 24, DestinationFamily: 4, DestinationIP: base.DestinationIP, DestinationPort: 443, Host: "a", NetworkCode: 1, ClassificationCode: 1},
		{SourceFamily: 4, SourceNetwork: base.SourceNetwork, SourcePrefixLen: 24, DestinationFamily: 4, DestinationIP: base.DestinationIP, DestinationPort: 443, Host: "a|b", NetworkCode: 2, ClassificationCode: 1},
	}
	for index, changed := range changes {
		if attribution.DimensionKey(base) == attribution.DimensionKey(changed) {
			t.Errorf("change %d did not change key", index)
		}
	}
	left := base
	left.Host = "a|b"
	right := base
	right.Host = "a"
	right.DestinationIP = []byte{'b', 0, 0, 0}
	if attribution.DimensionKey(left) == attribution.DimensionKey(right) {
		t.Fatal("length-ambiguous fields collided")
	}
}

func TestDimensionKeyDoesNotAliasCallerMemory(t *testing.T) {
	dimension := storage.FlowDimension{DestinationFamily: 4, DestinationIP: []byte{198, 51, 100, 7}, DestinationPort: 443, ClassificationCode: 1}
	key := attribution.DimensionKey(dimension)
	dimension.DestinationIP[0] = 203
	if key == attribution.DimensionKey(dimension) {
		t.Fatal("key followed caller mutation")
	}
	keys := []string{attribution.DimensionKey(dimension), key}
	sort.Strings(keys)
	if keys[0] == keys[1] {
		t.Fatal("sorted keys are not distinct")
	}
}

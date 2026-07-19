package attribution

import (
	"encoding/binary"

	"github.com/Willxup/flowlens/internal/storage"
)

// DimensionKey encodes every durable dimension field in a deterministic,
// length-delimited binary key suitable for sorting and map lookup.
func DimensionKey(dimension storage.FlowDimension) string {
	buffer := make([]byte, 0, 96+len(dimension.SourceNetwork)+len(dimension.DestinationIP)+len(dimension.Host))
	buffer = appendInt64(buffer, dimension.SourceFamily)
	buffer = appendBytes(buffer, dimension.SourceNetwork)
	buffer = appendInt64(buffer, dimension.SourcePrefixLen)
	buffer = appendInt64(buffer, dimension.DestinationFamily)
	buffer = appendBytes(buffer, dimension.DestinationIP)
	buffer = appendInt64(buffer, dimension.DestinationPort)
	buffer = appendBytes(buffer, []byte(dimension.Host))
	buffer = appendInt64(buffer, dimension.NetworkCode)
	buffer = appendInt64(buffer, dimension.ClassificationCode)
	return string(buffer)
}

func appendInt64(target []byte, value int64) []byte {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(value))
	return append(target, encoded[:]...)
}

func appendBytes(target []byte, value []byte) []byte {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	target = append(target, length[:]...)
	return append(target, value...)
}

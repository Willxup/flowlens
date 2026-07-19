package attribution

import (
	"net/netip"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Willxup/flowlens/internal/clashapi"
	"github.com/Willxup/flowlens/internal/storage"
)

// NormalizeDimension converts optional Clash metadata into one canonical,
// privacy-processed durable dimension. Invalid optional values become unknown.
func NormalizeDimension(connection clashapi.Connection, options Options) storage.FlowDimension {
	dimension := storage.FlowDimension{
		DestinationPort:    -1,
		ClassificationCode: 1,
	}
	normalizeSource(&dimension, connection.Metadata.SourceIP, options)
	if address, ok := parseAddress(connection.Metadata.DestinationIP); ok {
		dimension.DestinationFamily = addressFamily(address)
		dimension.DestinationIP = append([]byte(nil), address.AsSlice()...)
	}
	if port, err := strconv.ParseInt(strings.TrimSpace(connection.Metadata.DestinationPort), 10, 64); err == nil && port >= 1 && port <= 65535 {
		dimension.DestinationPort = port
	}
	switch strings.ToLower(strings.TrimSpace(connection.Metadata.Network)) {
	case "tcp":
		dimension.NetworkCode = 1
	case "udp":
		dimension.NetworkCode = 2
	}
	dimension.Host = normalizeHost(connection.Metadata.Host)
	return dimension
}

func normalizeSource(dimension *storage.FlowDimension, raw string, options Options) {
	if options.SourceMode == SourceDisabled {
		return
	}
	address, ok := parseAddress(raw)
	if !ok {
		return
	}
	prefix := address.BitLen()
	if options.SourceMode == SourcePrefix {
		if address.Is4() {
			prefix = options.IPv4Prefix
		} else {
			prefix = options.IPv6Prefix
		}
		if prefix < 0 || prefix > address.BitLen() {
			return
		}
		address = netip.PrefixFrom(address, prefix).Masked().Addr()
	} else if options.SourceMode != SourceFull {
		return
	}
	dimension.SourceFamily = addressFamily(address)
	dimension.SourceNetwork = append([]byte(nil), address.AsSlice()...)
	dimension.SourcePrefixLen = int64(prefix)
}

func parseAddress(raw string) (netip.Addr, bool) {
	address, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap(), true
}

func addressFamily(address netip.Addr) int64 {
	if address.Is4() {
		return 4
	}
	return 6
}

func normalizeHost(raw string) string {
	value := strings.TrimSpace(raw)
	if strings.HasSuffix(value, ".") {
		value = strings.TrimSuffix(value, ".")
	}
	value = strings.ToLower(value)
	if value == "" || len(value) > 253 || !utf8.ValidString(value) {
		return ""
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return ""
		}
	}
	return value
}

// EndpointValue returns a canonical IP:port key or an empty string when the
// destination cannot form an endpoint.
func EndpointValue(dimension storage.FlowDimension) string {
	if dimension.DestinationPort < 1 || dimension.DestinationPort > 65535 {
		return ""
	}
	address, ok := netip.AddrFromSlice(dimension.DestinationIP)
	if !ok || addressFamily(address.Unmap()) != dimension.DestinationFamily {
		return ""
	}
	return netip.AddrPortFrom(address.Unmap(), uint16(dimension.DestinationPort)).String()
}

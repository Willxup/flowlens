package attribution_test

import (
	"bytes"
	"testing"

	"github.com/Willxup/flowlens/internal/attribution"
	"github.com/Willxup/flowlens/internal/clashapi"
)

func TestNormalizeDimensionCanonicalizesConnectionMetadata(t *testing.T) {
	connection := clashapi.Connection{Metadata: clashapi.Metadata{
		SourceIP: "192.0.2.129", DestinationIP: "2001:0db8::20",
		DestinationPort: "443", Network: " TCP ", Host: " Example.TEST. ",
	}}
	dimension := attribution.NormalizeDimension(connection, attribution.Options{
		TopK: 20, SourceMode: attribution.SourcePrefix, IPv4Prefix: 24, IPv6Prefix: 64,
	})
	if dimension.SourceFamily != 4 || dimension.SourcePrefixLen != 24 ||
		!bytes.Equal(dimension.SourceNetwork, []byte{192, 0, 2, 0}) {
		t.Errorf("source = family:%d prefix:%d bytes:%v", dimension.SourceFamily, dimension.SourcePrefixLen, dimension.SourceNetwork)
	}
	if dimension.DestinationFamily != 6 || len(dimension.DestinationIP) != 16 ||
		dimension.DestinationIP[0] != 0x20 || dimension.DestinationIP[1] != 0x01 || dimension.DestinationIP[15] != 0x20 {
		t.Errorf("destination = family:%d bytes:%v", dimension.DestinationFamily, dimension.DestinationIP)
	}
	if dimension.DestinationPort != 443 || dimension.NetworkCode != 1 ||
		dimension.Host != "example.test" || dimension.ClassificationCode != 1 {
		t.Errorf("dimension = %#v", dimension)
	}
	if endpoint := attribution.EndpointValue(dimension); endpoint != "[2001:db8::20]:443" {
		t.Errorf("EndpointValue() = %q", endpoint)
	}
}

func TestNormalizeDimensionSourcePrivacyModes(t *testing.T) {
	connection := clashapi.Connection{Metadata: clashapi.Metadata{SourceIP: "2001:db8:1:2::9"}}
	tests := []struct {
		name       string
		mode       attribution.SourceMode
		family     int64
		prefix     int64
		wantLength int
		last       byte
	}{
		{name: "full", mode: attribution.SourceFull, family: 6, prefix: 128, wantLength: 16, last: 9},
		{name: "prefix", mode: attribution.SourcePrefix, family: 6, prefix: 64, wantLength: 16, last: 0},
		{name: "disabled", mode: attribution.SourceDisabled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dimension := attribution.NormalizeDimension(connection, attribution.Options{
				TopK: 20, SourceMode: test.mode, IPv4Prefix: 24, IPv6Prefix: 64,
			})
			if dimension.SourceFamily != test.family || dimension.SourcePrefixLen != test.prefix ||
				len(dimension.SourceNetwork) != test.wantLength {
				t.Fatalf("source = family:%d prefix:%d bytes:%v", dimension.SourceFamily, dimension.SourcePrefixLen, dimension.SourceNetwork)
			}
			if test.wantLength > 0 && dimension.SourceNetwork[len(dimension.SourceNetwork)-1] != test.last {
				t.Errorf("source last byte = %d", dimension.SourceNetwork[len(dimension.SourceNetwork)-1])
			}
		})
	}
}

func TestNormalizeDimensionDegradesMalformedOptionalFields(t *testing.T) {
	connection := clashapi.Connection{Metadata: clashapi.Metadata{
		SourceIP: "not-an-ip", DestinationIP: "also-bad", DestinationPort: "70000",
		Network: "quic", Host: "bad\nname",
	}}
	dimension := attribution.NormalizeDimension(connection, attribution.Options{
		TopK: 20, SourceMode: attribution.SourceFull, IPv4Prefix: 24, IPv6Prefix: 64,
	})
	if dimension.SourceFamily != 0 || len(dimension.SourceNetwork) != 0 ||
		dimension.DestinationFamily != 0 || len(dimension.DestinationIP) != 0 ||
		dimension.DestinationPort != -1 || dimension.NetworkCode != 0 || dimension.Host != "" {
		t.Errorf("malformed dimension = %#v", dimension)
	}
	if endpoint := attribution.EndpointValue(dimension); endpoint != "" {
		t.Errorf("EndpointValue() = %q", endpoint)
	}
}

package attribution

import "github.com/Willxup/flowlens/internal/clashapi"

// SourceMode controls how source addresses enter durable dimensions.
type SourceMode string

const (
	SourceFull     SourceMode = "full"
	SourcePrefix   SourceMode = "prefix"
	SourceDisabled SourceMode = "disabled"
)

// Options contains the bounded attribution and privacy policy.
type Options struct {
	TopK         int
	SourceMode   SourceMode
	IPv4Prefix   int
	IPv6Prefix   int
	Capabilities clashapi.DimensionCapabilities
}

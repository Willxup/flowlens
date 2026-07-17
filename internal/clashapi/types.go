package clashapi

// Version is the subset of the Clash version response used by FlowLens.
type Version struct {
	Version string `json:"version"`
	Premium bool   `json:"premium"`
	Meta    bool   `json:"meta"`
}

// ConnectionsSnapshot contains the global cumulative counters and active
// connections returned by the Clash API.
type ConnectionsSnapshot struct {
	DownloadTotal int64        `json:"downloadTotal"`
	UploadTotal   int64        `json:"uploadTotal"`
	Connections   []Connection `json:"connections"`
	Memory        int64        `json:"memory"`
}

// Connection contains the cumulative counters and metadata for one active
// Clash connection.
type Connection struct {
	ID       string   `json:"id"`
	Metadata Metadata `json:"metadata"`
	Upload   int64    `json:"upload"`
	Download int64    `json:"download"`
}

// Metadata preserves the optional upstream dimension values FlowLens probes.
// Clash encodes source and destination ports as strings.
type Metadata struct {
	Network         string `json:"network"`
	SourceIP        string `json:"sourceIP"`
	DestinationIP   string `json:"destinationIP"`
	SourcePort      string `json:"sourcePort"`
	DestinationPort string `json:"destinationPort"`
	Host            string `json:"host"`
}

// TrafficSample is one bytes-per-second sample from /traffic.
type TrafficSample struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// MemorySample is one memory sample from the optional /memory endpoint.
type MemorySample struct {
	Inuse   int64 `json:"inuse"`
	OSLimit int64 `json:"oslimit"`
}

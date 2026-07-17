package clashapi

import (
	"context"
	"errors"
	"strings"
)

// Capabilities reports the required and optional Clash API features observed
// during startup probing.
type Capabilities struct {
	Version      bool
	Traffic      bool
	Connections  bool
	Memory       bool
	VersionValue string
	Dimensions   DimensionCapabilities
}

// DimensionCapabilities reports which optional connection dimensions were
// observed across the startup snapshots.
type DimensionCapabilities struct {
	ConnectionID    bool
	SourceIP        bool
	DestinationIP   bool
	DestinationPort bool
	Network         bool
	Host            bool
}

// Probe validates required endpoints, records optional memory support, and
// derives the observable connection dimension matrix.
func (c *Client) Probe(ctx context.Context) (Capabilities, error) {
	version, err := c.probeVersion(ctx)
	if err != nil {
		return Capabilities{}, probeError("/version")
	}

	first, err := c.probeConnections(ctx)
	if err != nil {
		return Capabilities{}, probeError("/connections")
	}
	second, err := c.probeConnections(ctx)
	if err != nil {
		return Capabilities{}, probeError("/connections")
	}

	if err := c.probeTraffic(ctx); err != nil {
		return Capabilities{}, probeError("/traffic")
	}

	return Capabilities{
		Version:      true,
		Traffic:      true,
		Connections:  true,
		Memory:       c.probeMemory(ctx) == nil,
		VersionValue: version.Version,
		Dimensions:   detectDimensions(first, second),
	}, nil
}

func (c *Client) probeVersion(ctx context.Context) (Version, error) {
	operationContext, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	version, err := c.Version(operationContext)
	if err != nil || strings.TrimSpace(version.Version) == "" {
		return Version{}, errors.New("version capability unavailable")
	}
	return version, nil
}

func (c *Client) probeConnections(ctx context.Context) (ConnectionsSnapshot, error) {
	operationContext, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	return c.Connections(operationContext)
}

func (c *Client) probeTraffic(ctx context.Context) error {
	operationContext, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	stream, err := c.Traffic(operationContext)
	if err != nil {
		return err
	}
	_, nextErr := stream.Next()
	closeErr := stream.Close()
	if nextErr != nil {
		return nextErr
	}
	return closeErr
}

func (c *Client) probeMemory(ctx context.Context) error {
	operationContext, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	stream, err := c.Memory(operationContext)
	if err != nil {
		return err
	}
	_, nextErr := stream.Next()
	closeErr := stream.Close()
	if nextErr != nil {
		return nextErr
	}
	return closeErr
}

func detectDimensions(snapshots ...ConnectionsSnapshot) DimensionCapabilities {
	var dimensions DimensionCapabilities
	for _, snapshot := range snapshots {
		for _, connection := range snapshot.Connections {
			dimensions.ConnectionID = dimensions.ConnectionID || strings.TrimSpace(connection.ID) != ""
			dimensions.SourceIP = dimensions.SourceIP || strings.TrimSpace(connection.Metadata.SourceIP) != ""
			dimensions.DestinationIP = dimensions.DestinationIP || strings.TrimSpace(connection.Metadata.DestinationIP) != ""
			dimensions.DestinationPort = dimensions.DestinationPort || strings.TrimSpace(connection.Metadata.DestinationPort) != ""
			dimensions.Network = dimensions.Network || strings.TrimSpace(connection.Metadata.Network) != ""
			dimensions.Host = dimensions.Host || strings.TrimSpace(connection.Metadata.Host) != ""
		}
	}
	return dimensions
}

func probeError(endpoint string) error {
	return errors.New("Clash API probe failed at " + endpoint)
}

package query

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/0xkowalskidev/gameserverquery/protocol"
)

// Option is a functional option for configuring queries
type Option func(*protocol.Options)

// Query queries a server using the specified protocol
func Query(ctx context.Context, game, addr string, opts ...Option) (*protocol.ServerInfo, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	// Use the unified QueryEngine
	engine := NewQueryEngine()
	req := &QueryRequest{
		Type:    QueryTypeSingle,
		Address: addr,
		Game:    game,
		Options: options,
	}

	result := engine.Execute(ctx, req)
	if result.Error != nil {
		return nil, result.Error
	}

	if len(result.Servers) == 0 {
		return nil, fmt.Errorf("no responsive server found at %s", addr)
	}

	return result.Servers[0], nil
}

// AutoDetect tries to detect the game type by querying common protocols
func AutoDetect(ctx context.Context, addr string, opts ...Option) (*protocol.ServerInfo, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	// Use the unified QueryEngine
	engine := NewQueryEngine()
	req := &QueryRequest{
		Type:    QueryTypeAutoDetect,
		Address: addr,
		Options: options,
	}

	result := engine.Execute(ctx, req)
	if result.Error != nil {
		return nil, result.Error
	}

	if len(result.Servers) == 0 {
		return nil, fmt.Errorf("no responsive server found at %s", addr)
	}

	return result.Servers[0], nil
}

// DiscoverServers scans for multiple game servers on the given host
func DiscoverServers(ctx context.Context, addr string, opts ...Option) ([]*protocol.ServerInfo, error) {
	options := DefaultOptions()
	options.DiscoveryMode = true // Enable discovery mode for shorter timeouts
	for _, opt := range opts {
		opt(options)
	}

	// Use the unified QueryEngine
	engine := NewQueryEngine()
	req := &QueryRequest{
		Type:    QueryTypeDiscovery,
		Address: addr,
		Options: options,
	}

	result := engine.Execute(ctx, req)
	if result.Error != nil {
		return nil, result.Error
	}

	return result.Servers, nil
}

// ScanProgress represents the progress of a server scan
type ScanProgress struct {
	TotalPorts     int
	TotalProtocols int
	Completed      int
	ServersFound   int
}

// DiscoverServersWithProgress scans for multiple game servers and reports progress
func DiscoverServersWithProgress(ctx context.Context, addr string, progressChan chan<- ScanProgress, opts ...Option) ([]*protocol.ServerInfo, error) {
	options := DefaultOptions()
	options.DiscoveryMode = true // Enable discovery mode for shorter timeouts
	for _, opt := range opts {
		opt(options)
	}

	// Use the unified QueryEngine with progress callback
	engine := NewQueryEngine()

	// Create a callback function that forwards progress to the channel
	progressCallback := func(progress ScanProgress) {
		if progressChan != nil {
			select {
			case progressChan <- progress:
			default:
			}
		}
	}

	req := &QueryRequest{
		Type:             QueryTypeDiscovery,
		Address:          addr,
		Options:          options,
		ProgressCallback: progressCallback,
	}

	result := engine.Execute(ctx, req)

	// Close progress channel after all results are collected
	if progressChan != nil {
		close(progressChan)
	}

	if result.Error != nil {
		return nil, result.Error
	}

	return result.Servers, nil
}

// SupportedGames returns a list of supported game protocols including aliases
func SupportedGames() []string {
	return protocol.AllGameNames()
}

// DefaultPort returns the default port for a game
func DefaultPort(game string) int {
	if proto, exists := protocol.GetProtocol(game); exists {
		return proto.DefaultPort()
	}
	return 0
}

// DefaultQueryPort returns the default query port for a game
func DefaultQueryPort(game string) int {
	if proto, exists := protocol.GetProtocol(game); exists {
		return proto.DefaultQueryPort()
	}
	return 0
}

// Removed: Large helper functions moved to QueryEngine

// parseAddress parses an address string and returns host, port
func parseAddress(addr string, optPort, defaultPort int) (string, int, error) {
	if addr == "" {
		return "", 0, fmt.Errorf("address cannot be empty")
	}

	// Try to split host and port using Go's built-in function
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		// No port specified - check if it's IPv6 with brackets but no port
		if len(addr) > 2 && addr[0] == '[' && addr[len(addr)-1] == ']' {
			// Remove brackets from IPv6 address
			host = addr[1 : len(addr)-1]
		} else {
			host = addr
		}
		port := optPort
		if port == 0 {
			port = defaultPort
		}
		return host, port, nil
	}

	// Port was specified, parse it
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %s", portStr)
	}

	return host, port, nil
}

// Removed: setServerInfoFields moved to QueryEngine

// DefaultOptions returns default query options
func DefaultOptions() *protocol.Options {
	return &protocol.Options{
		Timeout:        5 * time.Second,
		Port:           0, // Use protocol default
		Players:        false,
		PortRange:      nil,
		MaxConcurrency: 0, // unlimited
		DiscoveryMode:  false,
	}
}

// Timeout sets the query timeout
func Timeout(d time.Duration) Option {
	return func(o *protocol.Options) {
		o.Timeout = d
	}
}

// Port sets a specific port to query
func Port(port int) Option {
	return func(o *protocol.Options) {
		o.Port = port
	}
}

// WithPlayers includes player list in the query
func WithPlayers() Option {
	return func(o *protocol.Options) {
		o.Players = true
	}
}

// WithPortRange specifies a range of ports to scan
func WithPortRange(start, end int) Option {
	return func(o *protocol.Options) {
		ports := make([]int, 0, end-start+1)
		for port := start; port <= end; port++ {
			ports = append(ports, port)
		}
		o.PortRange = ports
	}
}

// WithCustomPorts specifies exact ports to scan
func WithCustomPorts(ports []int) Option {
	return func(o *protocol.Options) {
		o.PortRange = ports
	}
}

// WithMaxConcurrency limits concurrent queries
func WithMaxConcurrency(max int) Option {
	return func(o *protocol.Options) {
		o.MaxConcurrency = max
	}
}

// WithDebug enables debug logging
func WithDebug() Option {
	return func(o *protocol.Options) {
		o.Debug = true
	}
}

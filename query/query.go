package query

import (
	"context"
	"fmt"
	"math"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/0xkowalskidev/gameserverquery/protocol"
)

// Option is a functional option for configuring queries
type Option func(*QueryOptions)

// QueryOptions holds all query configuration
type QueryOptions struct {
	Game           string
	Port           int
	Timeout        time.Duration
	Players        bool
	PortRange      []int
	MaxConcurrency int
	Debug          bool
}

// ScanProgress represents the progress of a server scan
type ScanProgress struct {
	TotalPorts     int
	TotalProtocols int
	Completed      int
	ServersFound   int
}

// Common game server ports - simplified hardcoded list
var commonPorts = []int{25565, 27015, 7777, 28015, 27016, 7778, 25564}

// Protocol order by popularity
var protocolOrder = []string{"minecraft", "a2s", "terraria"}

// Query queries a server with automatic game detection if no game specified
func Query(ctx context.Context, addr string, opts ...Option) (*protocol.ServerInfo, error) {
	options := &QueryOptions{
		Timeout: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(options)
	}

	if options.Debug {
		fmt.Printf("[DEBUG] Query: Starting query for '%s'\n", addr)
	}

	// Parse address
	host, port, err := parseAddress(addr, options.Port)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	// Try specific game first if provided
	if options.Game != "" {
		if options.Debug {
			fmt.Printf("[DEBUG] Query: Trying specific game '%s'\n", options.Game)
		}
		if info, err := trySpecificGame(ctx, options.Game, host, port, options); err == nil {
			return info, nil
		}
		if options.Debug {
			fmt.Printf("[DEBUG] Query: Specific game '%s' failed, trying auto-detect\n", options.Game)
		}
	}

	// Auto-detect: try protocols in order of popularity
	if options.Debug {
		fmt.Printf("[DEBUG] Query: Auto-detecting game type\n")
	}

	// Try exact port first
	if port > 0 {
		if info, err := tryPort(ctx, host, port, options); err == nil {
			return info, nil
		}
	}

	// Try common ports
	for _, testPort := range commonPorts {
		if testPort == port {
			continue // Already tried
		}
		if info, err := tryPort(ctx, host, testPort, options); err == nil {
			return info, nil
		}
	}

	return nil, fmt.Errorf("no responsive server found at %s", addr)
}

// DiscoverServers scans for multiple game servers on the given host
func DiscoverServers(ctx context.Context, addr string, opts ...Option) ([]*protocol.ServerInfo, error) {
	return discoverServers(ctx, addr, opts, nil)
}

// DiscoverServersWithProgress scans for multiple game servers and reports progress
func DiscoverServersWithProgress(ctx context.Context, addr string, progressChan chan<- ScanProgress, opts ...Option) ([]*protocol.ServerInfo, error) {
	defer func() {
		if progressChan != nil {
			close(progressChan)
		}
	}()

	progressCallback := func(progress ScanProgress) {
		if progressChan != nil {
			select {
			case progressChan <- progress:
			default:
			}
		}
	}

	return discoverServers(ctx, addr, opts, progressCallback)
}

// discoverServers is the internal implementation for server discovery
func discoverServers(ctx context.Context, addr string, opts []Option, progressCallback func(ScanProgress)) ([]*protocol.ServerInfo, error) {
	options := &QueryOptions{
		Timeout: 2 * time.Second, // Shorter timeout for discovery
	}
	for _, opt := range opts {
		opt(options)
	}

	if options.Debug {
		fmt.Printf("[DEBUG] Discovery: Starting discovery for '%s'\n", addr)
	}

	// Parse address
	host, specifiedPort, err := parseAddress(addr, options.Port)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	// Determine ports to scan
	var portsToScan []int
	if len(options.PortRange) > 0 {
		portsToScan = options.PortRange
	} else if specifiedPort > 0 {
		portsToScan = []int{specifiedPort}
	} else {
		portsToScan = commonPorts
	}

	if options.Debug {
		fmt.Printf("[DEBUG] Discovery: Scanning %d ports\n", len(portsToScan))
	}

	// Set up concurrency
	maxConcurrency := options.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 10 // Reasonable default
	}
	semaphore := make(chan struct{}, maxConcurrency)

	// Results collection
	results := make(chan *protocol.ServerInfo, len(portsToScan))
	var wg sync.WaitGroup
	var completed int
	var mu sync.Mutex

	// Send initial progress
	if progressCallback != nil {
		progressCallback(ScanProgress{
			TotalPorts:     len(portsToScan),
			TotalProtocols: len(protocolOrder), // Simple approximation
			Completed:      0,
			ServersFound:   0,
		})
	}

	// Scan each port
	for _, port := range portsToScan {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-semaphore }()

			if info, err := tryPort(ctx, host, port, options); err == nil {
				results <- info
			}

			// Update progress
			mu.Lock()
			completed++
			current := completed
			mu.Unlock()

			if progressCallback != nil {
				// Count current servers
				serversFound := 0
				select {
				case <-time.After(1 * time.Millisecond):
					// Non-blocking check
				default:
				}
				// Simple approximation for progress
				progressCallback(ScanProgress{
					TotalPorts:     len(portsToScan),
					TotalProtocols: len(protocolOrder),
					Completed:      current,
					ServersFound:   serversFound,
				})
			}
		}(port)
	}

	// Wait for completion
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var servers []*protocol.ServerInfo
	for info := range results {
		servers = append(servers, info)
	}

	if options.Debug {
		fmt.Printf("[DEBUG] Discovery: Found %d servers\n", len(servers))
	}

	return servers, nil
}

// trySpecificGame tries to query using a specific game protocol
func trySpecificGame(ctx context.Context, game, host string, port int, options *QueryOptions) (*protocol.ServerInfo, error) {
	gameConfig, proto, exists := protocol.GetGameConfigFromRegistry(game)
	if !exists {
		return nil, fmt.Errorf("unsupported game: %s", game)
	}

	// Use game's default port if none specified
	if port == 0 {
		port = gameConfig.QueryPort
	}

	return queryProtocol(ctx, proto, host, port, options)
}

// tryPort tries all protocols on a specific port
func tryPort(ctx context.Context, host string, port int, options *QueryOptions) (*protocol.ServerInfo, error) {
	if options.Debug {
		fmt.Printf("[DEBUG] Query: Trying port %d\n", port)
	}

	// Try protocols in order of popularity
	for _, protoName := range protocolOrder {
		if proto, exists := protocol.GetProtocol(protoName); exists {
			if info, err := queryProtocol(ctx, proto, host, port, options); err == nil {
				if options.Debug {
					fmt.Printf("[DEBUG] Query: SUCCESS with %s on port %d\n", proto.Name(), port)
				}
				return info, nil
			}
		}
	}

	// Try any remaining protocols
	for _, proto := range protocol.AllProtocols() {
		// Skip if already tried
		skip := false
		for _, tried := range protocolOrder {
			if proto.Name() == tried {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		if info, err := queryProtocol(ctx, proto, host, port, options); err == nil {
			if options.Debug {
				fmt.Printf("[DEBUG] Query: SUCCESS with %s on port %d\n", proto.Name(), port)
			}
			return info, nil
		}
	}

	return nil, fmt.Errorf("no protocol worked on port %d", port)
}

// queryProtocol queries a specific protocol on a host:port
func queryProtocol(ctx context.Context, proto protocol.Protocol, host string, port int, options *QueryOptions) (*protocol.ServerInfo, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()

	// Create protocol options
	protoOpts := &protocol.Options{
		Timeout: options.Timeout,
		Players: options.Players,
		Debug:   options.Debug,
	}

	info, err := proto.Query(ctx, addr, protoOpts)
	if err != nil {
		return nil, err
	}

	if !info.Online {
		return nil, fmt.Errorf("server offline")
	}

	// Set common fields
	info.Address = host
	info.Port = port
	info.QueryPort = port
	if info.Ping == 0 {
		info.Ping = int(math.Ceil(float64(time.Since(start).Nanoseconds()) / 1e6))
	}

	return info, nil
}

// parseAddress parses an address string and returns host, port
func parseAddress(addr string, optPort int) (string, int, error) {
	if addr == "" {
		return "", 0, fmt.Errorf("address cannot be empty")
	}

	// Try to split host and port
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		// No port specified
		if len(addr) > 2 && addr[0] == '[' && addr[len(addr)-1] == ']' {
			// Remove brackets from IPv6 address
			host = addr[1 : len(addr)-1]
		} else {
			host = addr
		}
		return host, optPort, nil
	}

	// Port was specified, parse it
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %s", portStr)
	}

	return host, port, nil
}

// Utility functions

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

// Option functions

// WithGame specifies which game protocol to try first
func WithGame(game string) Option {
	return func(o *QueryOptions) {
		o.Game = game
	}
}

// WithPort sets a specific port to query
func WithPort(port int) Option {
	return func(o *QueryOptions) {
		o.Port = port
	}
}

// WithTimeout sets the query timeout
func WithTimeout(d time.Duration) Option {
	return func(o *QueryOptions) {
		o.Timeout = d
	}
}

// WithPlayers includes player list in the query
func WithPlayers() Option {
	return func(o *QueryOptions) {
		o.Players = true
	}
}

// WithPortRange specifies a range of ports to scan
func WithPortRange(start, end int) Option {
	return func(o *QueryOptions) {
		ports := make([]int, 0, end-start+1)
		for port := start; port <= end; port++ {
			ports = append(ports, port)
		}
		o.PortRange = ports
	}
}

// WithPorts specifies exact ports to scan
func WithPorts(ports []int) Option {
	return func(o *QueryOptions) {
		o.PortRange = ports
	}
}

// WithMaxConcurrency limits concurrent queries
func WithMaxConcurrency(max int) Option {
	return func(o *QueryOptions) {
		o.MaxConcurrency = max
	}
}

// WithDebug enables debug logging
func WithDebug() Option {
	return func(o *QueryOptions) {
		o.Debug = true
	}
}


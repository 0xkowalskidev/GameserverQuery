package query

import (
	"context"
	"fmt"
	"math"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/0xkowalskidev/gameserverquery/protocol"
)

// Option is a functional option for configuring queries
type Option func(*protocol.Options)

// ScanProgress represents the progress of a server scan
type ScanProgress struct {
	TotalPorts     int
	TotalProtocols int
	Completed      int
	ServersFound   int
}

// Query queries a server using the specified protocol
func Query(ctx context.Context, game, addr string, opts ...Option) (*protocol.ServerInfo, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	if options.Debug {
		debugLogf("Query", "Starting single query for game '%s' at address '%s'", game, addr)
	}

	// Get game config and protocol
	gameConfig, proto, exists := protocol.GetGameConfigFromRegistry(game)
	if !exists {
		if options.Debug {
			debugLogf("Query", "Unsupported game: %s", game)
		}
		return nil, fmt.Errorf("unsupported game: %s", game)
	}

	// Parse address and determine port - use game's query port by default
	host, requestedPort, err := parseAddress(addr, options.Port, gameConfig.QueryPort)
	if err != nil {
		if options.Debug {
			debugLogf("Query", "Address parsing failed: %v", err)
		}
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	if options.Debug {
		debugLogf("Query", "Parsed address - host: %s, requested port: %d, protocol: %s", host, requestedPort, proto.Name())
	}

	// Try the main port first
	if options.Debug {
		debugLogf("Query", "Trying primary port %d with %s protocol", requestedPort, proto.Name())
	}

	info, err := queryProtocol(ctx, proto, host, requestedPort, requestedPort, options)
	if err == nil && info.Online {
		if options.Debug {
			debugLogf("Query", "SUCCESS on primary port %d", requestedPort)
		}
		return info, nil
	}

	if options.Debug {
		debugLogf("Query", "Primary port %d failed: %v", requestedPort, err)
	}

	// Try adjacent ports with reduced timeout
	adjacentPorts := getAdjacentPorts(requestedPort)
	if options.Debug {
		debugLogf("Query", "Trying %d adjacent ports", len(adjacentPorts))
	}

	discoveryOptions := createDiscoveryOptions(options)
	for i, testPort := range adjacentPorts {
		if options.Debug {
			debugLogf("Query", "Trying adjacent port %d (%d/%d)", testPort, i+1, len(adjacentPorts))
		}

		// Try all protocols on this port
		for _, tryProto := range []protocol.Protocol{proto} {
			if info, err := queryProtocol(ctx, tryProto, host, requestedPort, testPort, discoveryOptions); err == nil && info.Online {
				if options.Debug {
					debugLogf("Query", "SUCCESS on adjacent port %d with %s", testPort, tryProto.Name())
				}
				return info, nil
			}
		}
	}

	if options.Debug {
		debugLogf("Query", "All ports failed, no responsive server found")
	}
	return nil, fmt.Errorf("no responsive server found at %s or adjacent ports", addr)
}

// AutoDetect tries to detect the game type by querying common protocols
func AutoDetect(ctx context.Context, addr string, opts ...Option) (*protocol.ServerInfo, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	if options.Debug {
		debugLogf("AutoDetect", "Starting auto-detection for address '%s'", addr)
	}

	host, port, err := parseAddress(addr, options.Port, 0)
	if err != nil {
		if options.Debug {
			debugLogf("AutoDetect", "Address parsing failed: %v", err)
		}
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	if options.Debug {
		debugLogf("AutoDetect", "Parsed address - host: %s, port: %d", host, port)
	}

	// If port is specified, try protocols in order of likelihood for that port
	if port != 0 {
		protocols := getProtocolsForPort(port)
		if options.Debug {
			debugLogf("AutoDetect", "Port %d specified, trying %d matching protocols first", port, len(protocols))
		}

		for i, proto := range protocols {
			if options.Debug {
				debugLogf("AutoDetect", "Trying protocol %s on port %d (%d/%d)", proto.Name(), port, i+1, len(protocols))
			}

			info, err := queryProtocol(ctx, proto, host, port, port, options)
			if err == nil && info.Online {
				if options.Debug {
					debugLogf("AutoDetect", "SUCCESS with %s on port %d", proto.Name(), port)
				}
				return info, nil
			}
			if options.Debug {
				debugLogf("AutoDetect", "FAILED with %s on port %d: %v", proto.Name(), port, err)
			}
		}

		// Try adjacent ports
		adjacentPorts := getAdjacentPorts(port)
		if options.Debug {
			debugLogf("AutoDetect", "Trying %d adjacent ports", len(adjacentPorts))
		}

		discoveryOptions := createDiscoveryOptions(options)
		for _, testPort := range adjacentPorts {
			if options.Debug {
				debugLogf("AutoDetect", "Trying adjacent port %d", testPort)
			}

			for _, proto := range getProtocolsForPort(testPort) {
				if info, err := queryProtocol(ctx, proto, host, port, testPort, discoveryOptions); err == nil && info.Online {
					if options.Debug {
						debugLogf("AutoDetect", "SUCCESS on adjacent port %d with %s", testPort, proto.Name())
					}
					return info, nil
				}
			}
		}
	}

	// Try all protocols on their default ports
	protocols := getProtocolsByPopularity()
	if options.Debug {
		debugLogf("AutoDetect", "Trying %d protocols on their default ports", len(protocols))
	}

	for i, proto := range protocols {
		testPort := port
		if testPort == 0 {
			testPort = proto.DefaultQueryPort()
		}

		if options.Debug {
			debugLogf("AutoDetect", "Trying protocol %s on default port %d (%d/%d)", proto.Name(), testPort, i+1, len(protocols))
		}

		info, err := queryProtocol(ctx, proto, host, port, testPort, options)
		if err == nil && info.Online {
			if options.Debug {
				debugLogf("AutoDetect", "SUCCESS with %s on default port %d", proto.Name(), testPort)
			}
			return info, nil
		}
		if options.Debug {
			debugLogf("AutoDetect", "FAILED with %s on default port %d: %v", proto.Name(), testPort, err)
		}
	}

	if options.Debug {
		debugLogf("AutoDetect", "All protocols failed, no responsive server found")
	}
	return nil, fmt.Errorf("no responsive server found at %s", addr)
}

// DiscoverServers scans for multiple game servers on the given host
func DiscoverServers(ctx context.Context, addr string, opts ...Option) ([]*protocol.ServerInfo, error) {
	options := DefaultOptions()
	options.DiscoveryMode = true // Enable discovery mode for shorter timeouts
	for _, opt := range opts {
		opt(options)
	}

	return discoverServers(ctx, addr, options, nil)
}

// DiscoverServersWithProgress scans for multiple game servers and reports progress
func DiscoverServersWithProgress(ctx context.Context, addr string, progressChan chan<- ScanProgress, opts ...Option) ([]*protocol.ServerInfo, error) {
	options := DefaultOptions()
	options.DiscoveryMode = true // Enable discovery mode for shorter timeouts
	for _, opt := range opts {
		opt(options)
	}

	// Create a callback function that forwards progress to the channel
	progressCallback := func(progress ScanProgress) {
		if progressChan != nil {
			select {
			case progressChan <- progress:
			default:
			}
		}
	}

	defer func() {
		if progressChan != nil {
			close(progressChan)
		}
	}()

	return discoverServers(ctx, addr, options, progressCallback)
}

// discoverServers is the internal implementation for server discovery
func discoverServers(ctx context.Context, addr string, options *protocol.Options, progressCallback func(ScanProgress)) ([]*protocol.ServerInfo, error) {
	if options.Debug {
		debugLogf("Discovery", "Starting server discovery for address '%s'", addr)
	}

	host, specifiedPort, err := parseAddress(addr, options.Port, 0)
	if err != nil {
		if options.Debug {
			debugLogf("Discovery", "Address parsing failed: %v", err)
		}
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	if options.Debug {
		debugLogf("Discovery", "Parsed address - host: %s, port: %d", host, specifiedPort)
	}

	// Get ports to scan
	var portsToScan []int
	if len(options.PortRange) > 0 {
		portsToScan = options.PortRange
		if options.Debug {
			debugLogf("Discovery", "Using custom port range: %v", options.PortRange)
		}
	} else if specifiedPort != 0 {
		portsToScan = []int{specifiedPort}
		if options.Debug {
			debugLogf("Discovery", "Using specified port: %d", specifiedPort)
		}
	} else {
		portsToScan = getDiscoveryPorts()
		if options.Debug {
			debugLogf("Discovery", "Using %d common game ports", len(portsToScan))
		}
	}

	if options.Debug {
		debugLogf("Discovery", "Will scan %d ports", len(portsToScan))
	}

	// Set up concurrency control
	maxConcurrency := options.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = len(portsToScan) * len(protocol.AllProtocols())
	}
	semaphore := make(chan struct{}, maxConcurrency)

	if options.Debug {
		debugLogf("Discovery", "Using concurrency limit: %d", maxConcurrency)
	}

	// Results channel and wait group
	type result struct {
		info *protocol.ServerInfo
		err  error
	}
	results := make(chan result, len(portsToScan)*len(protocol.AllProtocols()))
	var wg sync.WaitGroup

	// Progress tracking
	totalProtocols := len(protocol.AllProtocols())
	var progressMux sync.Mutex
	var completed, serversFound int

	// Send initial progress
	if progressCallback != nil {
		progressCallback(ScanProgress{
			TotalPorts:     len(portsToScan),
			TotalProtocols: totalProtocols,
			Completed:      0,
			ServersFound:   0,
		})
	}

	// Try protocols sequentially for each port
	for _, port := range portsToScan {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()

			// Try each protocol on this port until one succeeds
			for _, proto := range protocol.AllProtocols() {
				// Acquire semaphore
				select {
				case semaphore <- struct{}{}:
				case <-ctx.Done():
					return
				}

				found := false
				func() {
					defer func() { <-semaphore }()

					start := time.Now()
					info, err := queryProtocol(ctx, proto, host, port, port, options)

					// Update progress
					progressMux.Lock()
					completed++
					if err == nil && info.Online {
						serversFound++
						results <- result{info: info}
						found = true
					}
					currentProgress := ScanProgress{
						TotalPorts:     len(portsToScan),
						TotalProtocols: totalProtocols,
						Completed:      completed,
						ServersFound:   serversFound,
					}
					progressMux.Unlock()

					// Send progress update
					if progressCallback != nil {
						progressCallback(currentProgress)
					}

					if options.Debug && err == nil && info.Online {
						debugLogf("Discovery", "Found server on port %d with %s (took %v)", port, proto.Name(), time.Since(start))
					}
				}()

				if found {
					break // Found a working server, stop trying other protocols
				}
			}
		}(port)
	}

	// Wait for all queries to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect successful results
	var servers []*protocol.ServerInfo
	for res := range results {
		if res.info != nil {
			servers = append(servers, res.info)
		}
	}

	if options.Debug {
		debugLogf("Discovery", "Discovery complete, found %d servers", len(servers))
	}

	return servers, nil
}

// Helper functions

// queryProtocol queries a protocol on a specific host and port
func queryProtocol(ctx context.Context, proto protocol.Protocol, host string, requestedPort int, queryPort int, options *protocol.Options) (*protocol.ServerInfo, error) {
	testAddr := net.JoinHostPort(host, strconv.Itoa(queryPort))
	start := time.Now()

	if options.Debug {
		debugLogf("Query", "Querying %s with %s protocol", testAddr, proto.Name())
	}

	info, err := proto.Query(ctx, testAddr, options)
	elapsed := time.Since(start)

	if err != nil {
		if options.Debug {
			debugLogf("Query", "Query failed for %s (%s): %v (took %v)", testAddr, proto.Name(), err, elapsed)
		}
		return nil, err
	}

	if info.Online {
		setServerInfoFields(info, host, requestedPort, queryPort, start)
		if options.Debug {
			debugLogf("Query", "Query successful for %s (%s): online=%v, players=%d/%d (took %v)",
				testAddr, proto.Name(), info.Online, info.Players.Current, info.Players.Max, elapsed)
		}
	} else {
		if options.Debug {
			debugLogf("Query", "Server %s (%s) is offline (took %v)", testAddr, proto.Name(), elapsed)
		}
	}

	return info, nil
}

// setServerInfoFields sets common fields on ServerInfo
func setServerInfoFields(info *protocol.ServerInfo, host string, requestedPort int, queryPort int, start time.Time) {
	info.Address = host
	info.Port = requestedPort
	info.QueryPort = queryPort

	// Only set ping if the protocol didn't provide one (ping == 0)
	if info.Ping == 0 {
		info.Ping = int(math.Ceil(float64(time.Since(start).Nanoseconds()) / 1e6))
	}
}

// getAdjacentPorts returns ports adjacent to the given port
func getAdjacentPorts(port int) []int {
	const adjacentRange = 3
	var ports []int

	for offset := 1; offset <= adjacentRange; offset++ {
		// Try port + offset
		if testPort := port + offset; testPort <= 65535 {
			ports = append(ports, testPort)
		}
		// Try port - offset
		if testPort := port - offset; testPort >= 1024 {
			ports = append(ports, testPort)
		}
	}

	return ports
}

// getProtocolsForPort returns protocols ordered by likelihood for the given port
func getProtocolsForPort(port int) []protocol.Protocol {
	allProtocols := protocol.AllProtocols()
	var ordered, remaining []protocol.Protocol
	seen := make(map[string]bool)

	// First, try protocols that have games matching this port
	for _, proto := range allProtocols {
		if seen[proto.Name()] {
			continue
		}

		// Check if any game config matches this port
		hasMatch := false
		if proto.DefaultQueryPort() == port || proto.DefaultPort() == port {
			hasMatch = true
		} else {
			for _, game := range proto.Games() {
				if game.QueryPort == port || game.GamePort == port {
					hasMatch = true
					break
				}
			}
		}

		if hasMatch {
			ordered = append(ordered, proto)
			seen[proto.Name()] = true
		} else {
			remaining = append(remaining, proto)
		}
	}

	// Then try remaining protocols
	return append(ordered, remaining...)
}

// getProtocolsByPopularity returns protocols ordered by general popularity
func getProtocolsByPopularity() []protocol.Protocol {
	// Ordered by general popularity and likelihood of being found
	popularityOrder := []string{
		"minecraft", // Very common
		"a2s",       // Covers many Steam games
		"terraria",  // Popular indie game
	}

	var result []protocol.Protocol
	used := make(map[string]bool)

	// Add protocols in popularity order
	for _, name := range popularityOrder {
		if proto, exists := protocol.GetProtocol(name); exists {
			result = append(result, proto)
			used[name] = true
		}
	}

	// Add any remaining protocols
	for _, proto := range protocol.AllProtocols() {
		if !used[proto.Name()] {
			result = append(result, proto)
		}
	}

	return result
}

// getDiscoveryPorts returns common game server ports for discovery
func getDiscoveryPorts() []int {
	// Collect unique ports from all game configurations
	portMap := make(map[int]bool)
	for _, proto := range protocol.AllProtocols() {
		// Add the default protocol ports
		portMap[proto.DefaultQueryPort()] = true
		portMap[proto.DefaultPort()] = true

		// Add ports from all game configurations
		for _, game := range proto.Games() {
			portMap[game.QueryPort] = true
			portMap[game.GamePort] = true
		}
	}

	var ports []int
	for port := range portMap {
		ports = append(ports, port)
	}

	// Sort for consistent ordering
	sort.Ints(ports)
	return ports
}

// createDiscoveryOptions creates options optimized for discovery
func createDiscoveryOptions(baseOptions *protocol.Options) *protocol.Options {
	discoveryOptions := *baseOptions
	discoveryOptions.DiscoveryMode = true
	return &discoveryOptions
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

// Debug logging helpers
func debugLogf(component, format string, args ...interface{}) {
	fmt.Printf("[DEBUG %s] %s: %s\n", time.Now().Format("15:04:05.000"), component, fmt.Sprintf(format, args...))
}
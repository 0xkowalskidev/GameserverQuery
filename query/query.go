package query

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync"
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

	proto, exists := protocol.GetProtocol(game)
	if !exists {
		return nil, fmt.Errorf("unsupported game: %s", game)
	}

	// Parse address and determine port
	host, port, err := parseAddress(addr, options.Port, proto.DefaultPort())
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	// Try the specified port first
	info, err := queryWithServerInfo(ctx, proto, host, port, options)
	if err == nil && info.Online {
		return info, nil
	}

	// If that failed, try adjacent ports (±3 ports)
	const adjacentPortRange = 3
	discoveryOptions := createDiscoveryOptions(options)
	
	for offset := 1; offset <= adjacentPortRange; offset++ {
		// Try port + offset
		testPort := port + offset
		if testPort <= 65535 {
			if info, err := tryProtocolsOnPort(ctx, host, testPort, discoveryOptions); err == nil {
				return info, nil
			}
		}

		// Try port - offset
		testPort = port - offset
		if testPort >= 1024 {
			if info, err := tryProtocolsOnPort(ctx, host, testPort, discoveryOptions); err == nil {
				return info, nil
			}
		}
	}

	// If all attempts failed, return the original error
	return nil, fmt.Errorf("no responsive server found at %s or adjacent ports", addr)
}

// AutoDetect tries to detect the game type by querying common protocols
func AutoDetect(ctx context.Context, addr string, opts ...Option) (*protocol.ServerInfo, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	host, port, err := parseAddress(addr, options.Port, 0)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	// If port is specified, try to match it to a known default port
	if port != 0 {
		for name, proto := range protocol.AllProtocols() {
			if port == proto.DefaultPort() {
				info, err := Query(ctx, name, addr, opts...)
				if err == nil && info.Online {
					return info, nil
				}
			}
		}
	}

	// Try common games in order of popularity
	games := []string{"minecraft", "source", "terraria", "valheim", "rust", "ark-survival-evolved", "7-days-to-die", "project-zomboid", "satisfactory"}
	
	for _, game := range games {
		if proto, exists := protocol.GetProtocol(game); exists {
			testPort := port
			if testPort == 0 {
				testPort = proto.DefaultPort()
			}
			
			info, err := queryWithServerInfo(ctx, proto, host, testPort, options)
			if err == nil && info.Online {
				return info, nil
			}
		}
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

	host, specifiedPort, err := parseAddress(addr, options.Port, 0)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	// Determine which ports to scan
	var portsToScan []int
	if len(options.PortRange) > 0 || specifiedPort != 0 {
		// Use provided ports as-is
		portsToScan = determinePortsToScan(options, specifiedPort)
		if options.Debug {
			fmt.Printf("[DEBUG] Using specified ports: %v\n", portsToScan)
		}
	} else {
		// Dynamic discovery mode
		if options.Debug {
			fmt.Printf("[DEBUG] Starting dynamic port discovery for %s\n", host)
		}
		portsToScan = discoverPortsDynamically(ctx, host, options)
		if options.Debug {
			fmt.Printf("[DEBUG] Dynamic discovery found ports: %v\n", portsToScan)
		}
	}

	// Set up concurrency control
	maxConcurrency := options.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = len(portsToScan) * len(protocol.AllProtocols())
	}
	semaphore := make(chan struct{}, maxConcurrency)

	// Results channel and wait group
	type result struct {
		info *protocol.ServerInfo
		err  error
	}
	results := make(chan result, len(portsToScan)*len(protocol.AllProtocols()))
	var wg sync.WaitGroup

	// Try protocols sequentially for each port to avoid timeouts on wrong protocols
	for _, port := range portsToScan {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			
			// Try each protocol on this port until one succeeds
			testAddr := net.JoinHostPort(host, strconv.Itoa(port))
			
			for _, proto := range protocol.AllProtocols() {
				// Acquire semaphore
				select {
				case semaphore <- struct{}{}:
				case <-ctx.Done():
					if options.Debug {
						fmt.Printf("[DEBUG] Context cancelled while waiting for semaphore on port %d\n", port)
					}
					return
				}
				
				found := false
				func() {
					defer func() { <-semaphore }()
					
					if options.Debug {
						fmt.Printf("[DEBUG] Trying protocol %s on port %d\n", proto.Name(), port)
					}
					
					start := time.Now()
					info, err := proto.Query(ctx, testAddr, options)
					if err == nil && info.Online {
						if options.Debug {
							fmt.Printf("[DEBUG] SUCCESS: Found %s server on port %d (took %v)\n", proto.Name(), port, time.Since(start))
						}
						setServerInfoFields(info, host, port, start)
						results <- result{info: info}
						found = true
					} else if options.Debug {
						fmt.Printf("[DEBUG] FAIL: Protocol %s on port %d failed (took %v): %v\n", proto.Name(), port, time.Since(start), err)
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

	return servers, nil
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

	host, specifiedPort, err := parseAddress(addr, options.Port, 0)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	// Determine which ports to scan
	var portsToScan []int
	if len(options.PortRange) > 0 || specifiedPort != 0 {
		// Use provided ports as-is
		portsToScan = determinePortsToScan(options, specifiedPort)
		if options.Debug {
			fmt.Printf("[DEBUG] Using specified ports: %v\n", portsToScan)
		}
	} else {
		// Dynamic discovery mode with progress tracking
		if options.Debug {
			fmt.Printf("[DEBUG] Starting dynamic port discovery for %s\n", host)
		}
		
		if progressChan != nil {
			// Send initial discovery progress
			progressChan <- ScanProgress{
				TotalPorts:     0, // Unknown at this point
				TotalProtocols: len(protocol.AllProtocols()),
				Completed:      0,
				ServersFound:   0,
			}
		}
		
		portsToScan = discoverPortsDynamicallyWithProgress(ctx, host, options, progressChan)
		if options.Debug {
			fmt.Printf("[DEBUG] Dynamic discovery found ports: %v\n", portsToScan)
		}
	}

	// Set up concurrency control
	maxConcurrency := options.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = len(portsToScan) * len(protocol.AllProtocols())
	}
	semaphore := make(chan struct{}, maxConcurrency)

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
	if progressChan != nil {
		progressChan <- ScanProgress{
			TotalPorts:     len(portsToScan),
			TotalProtocols: totalProtocols,
			Completed:      0,
			ServersFound:   0,
		}
	}
	
	// Try protocols sequentially for each port to avoid timeouts on wrong protocols
	for _, port := range portsToScan {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			
			// Try each protocol on this port until one succeeds
			testAddr := net.JoinHostPort(host, strconv.Itoa(port))
			
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
					info, err := proto.Query(ctx, testAddr, options)
					
					// Update progress
					progressMux.Lock()
					completed++
					if err == nil && info.Online {
						serversFound++
						setServerInfoFields(info, host, port, start)
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
					if progressChan != nil {
						select {
						case progressChan <- currentProgress:
						default:
						}
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

	// Close progress channel after all results are collected
	if progressChan != nil {
		close(progressChan)
	}

	return servers, nil
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

// discoverPortsDynamicallyWithProgress expands from default ports and reports progress
func discoverPortsDynamicallyWithProgress(ctx context.Context, host string, options *protocol.Options, progressChan chan<- ScanProgress) []int {
	const deadPortThreshold = 3 // Stop after 3 consecutive ports with no servers
	const minPort = 1024        // Don't scan below this
	const maxPort = 65535       // Don't scan above this

	// Get unique default ports as seeds (avoid duplicate scanning)
	seedPorts := make(map[int]bool)
	for _, proto := range protocol.AllProtocols() {
		seedPorts[proto.DefaultPort()] = true
	}
	
	// Early exit optimization: track which seed ports have servers
	seedPortsWithServers := make(map[int]bool)
	
	// Track all ports we'll scan (to avoid duplicate scanning)
	allPorts := make(map[int]bool)
	var portsChecked, serversFound int
	var portsMux, progressMux sync.Mutex
	
	// Concurrent discovery with worker pool
	const maxDiscoveryWorkers = 3
	semaphore := make(chan struct{}, maxDiscoveryWorkers)
	var wg sync.WaitGroup
	
	// For each unique seed port, expand outward concurrently
	for seedPort := range seedPorts {
		wg.Add(1)
		go func(seedPort int) {
			defer wg.Done()
			
			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			
			// Check the seed port and expand from there regardless of result
			
			// Scan the seed port itself
			if options.Debug {
				fmt.Printf("[DEBUG] Checking seed port %d\n", seedPort)
			}
			
			progressMux.Lock()
			portsChecked++
			currentChecked := portsChecked
			currentFound := serversFound
			progressMux.Unlock()
			
			if progressChan != nil {
				progressChan <- ScanProgress{
					TotalPorts:     0, // Still discovering
					TotalProtocols: len(protocol.AllProtocols()),
					Completed:      currentChecked,
					ServersFound:   currentFound,
				}
			}
			
			if hasActiveServer(ctx, host, seedPort, options) {
				portsMux.Lock()
				if !allPorts[seedPort] {
					allPorts[seedPort] = true
					seedPortsWithServers[seedPort] = true
					progressMux.Lock()
					serversFound++
					progressMux.Unlock()
				}
				portsMux.Unlock()
				
				if options.Debug {
					fmt.Printf("[DEBUG] Found server on seed port %d\n", seedPort)
				}
			} else if options.Debug {
				fmt.Printf("[DEBUG] No server on seed port %d\n", seedPort)
			}
		
			// Scan upward from seed (always scan a few ports even if seed failed)
			consecutiveFailures := 0
			for port := seedPort + 1; port <= maxPort; port++ {
				// Skip if we've already checked this port from another seed
				portsMux.Lock()
				alreadyChecked := allPorts[port]
				portsMux.Unlock()
				
				if alreadyChecked {
					consecutiveFailures = 0 // Reset since we know there's a server here
					continue
				}
				
				progressMux.Lock()
				portsChecked++
				currentChecked := portsChecked
				currentFound := serversFound
				progressMux.Unlock()
				
				if progressChan != nil {
					progressChan <- ScanProgress{
						TotalPorts:     0, // Still discovering
						TotalProtocols: len(protocol.AllProtocols()),
						Completed:      currentChecked,
						ServersFound:   currentFound,
					}
				}
				
				// Quick check if any protocol responds on this port
				if hasActiveServer(ctx, host, port, options) {
					portsMux.Lock()
					if !allPorts[port] {
						allPorts[port] = true
						progressMux.Lock()
						serversFound++
						progressMux.Unlock()
					}
					portsMux.Unlock()
					consecutiveFailures = 0
				} else {
					consecutiveFailures++
					if consecutiveFailures >= deadPortThreshold {
						break
					}
				}
			}
			
			// Scan downward from seed (always scan a few ports even if seed failed)
			consecutiveFailures = 0
			for port := seedPort - 1; port >= minPort; port-- {
				// Skip if we've already checked this port from another seed
				portsMux.Lock()
				alreadyChecked := allPorts[port]
				portsMux.Unlock()
				
				if alreadyChecked {
					consecutiveFailures = 0 // Reset since we know there's a server here
					continue
				}
				
				progressMux.Lock()
				portsChecked++
				currentChecked := portsChecked
				currentFound := serversFound
				progressMux.Unlock()
				
				if progressChan != nil {
					progressChan <- ScanProgress{
						TotalPorts:     0, // Still discovering
						TotalProtocols: len(protocol.AllProtocols()),
						Completed:      currentChecked,
						ServersFound:   currentFound,
					}
				}
				
				if hasActiveServer(ctx, host, port, options) {
					portsMux.Lock()
					if !allPorts[port] {
						allPorts[port] = true
						progressMux.Lock()
						serversFound++
						progressMux.Unlock()
					}
					portsMux.Unlock()
					consecutiveFailures = 0
				} else {
					consecutiveFailures++
					if consecutiveFailures >= deadPortThreshold {
						break
					}
				}
			}
		}(seedPort)
	}
	
	// Wait for all discovery workers to complete
	wg.Wait()
	
	// Convert map to sorted slice
	var ports []int
	for port := range allPorts {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	
	return ports
}

// discoverPortsDynamically expands from default ports to find clusters of game servers
func discoverPortsDynamically(ctx context.Context, host string, options *protocol.Options) []int {
	const deadPortThreshold = 3 // Stop after 3 consecutive ports with no servers
	const minPort = 1024        // Don't scan below this
	const maxPort = 65535       // Don't scan above this

	// Get unique default ports as seeds (avoid duplicate scanning)
	seedPorts := make(map[int]bool)
	for _, proto := range protocol.AllProtocols() {
		seedPorts[proto.DefaultPort()] = true
	}
	

	// Track all ports we'll scan (to avoid duplicate scanning)
	allPorts := make(map[int]bool)
	
	// For each unique seed port, expand outward
	for seedPort := range seedPorts {
		// Check the seed port and expand from there regardless of result
		
		// Scan the seed port itself
		if options.Debug {
			fmt.Printf("[DEBUG] Checking seed port %d\n", seedPort)
		}
		if hasActiveServer(ctx, host, seedPort, options) {
			allPorts[seedPort] = true
			if options.Debug {
				fmt.Printf("[DEBUG] Found server on seed port %d\n", seedPort)
			}
		} else if options.Debug {
			fmt.Printf("[DEBUG] No server on seed port %d\n", seedPort)
		}
		
		// Scan upward from seed (always scan a few ports even if seed failed)
		consecutiveFailures := 0
		for port := seedPort + 1; port <= maxPort; port++ {
			// Skip if we've already checked this port from another seed
			if allPorts[port] {
				consecutiveFailures = 0 // Reset since we know there's a server here
				continue
			}
			
			// Quick check if any protocol responds on this port
			if hasActiveServer(ctx, host, port, options) {
				allPorts[port] = true
				consecutiveFailures = 0
			} else {
				consecutiveFailures++
				if consecutiveFailures >= deadPortThreshold {
					break
				}
			}
		}
		
		// Scan downward from seed (always scan a few ports even if seed failed)
		consecutiveFailures = 0
		for port := seedPort - 1; port >= minPort; port-- {
			// Skip if we've already checked this port from another seed
			if allPorts[port] {
				consecutiveFailures = 0 // Reset since we know there's a server here
				continue
			}
			
			if hasActiveServer(ctx, host, port, options) {
				allPorts[port] = true
				consecutiveFailures = 0
			} else {
				consecutiveFailures++
				if consecutiveFailures >= deadPortThreshold {
					break
				}
			}
		}
	}
	
	// Convert map to sorted slice
	var ports []int
	for port := range allPorts {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	
	return ports
}

// hasActiveServer checks if any protocol responds on the given port
func hasActiveServer(ctx context.Context, host string, port int, options *protocol.Options) bool {
	// Use discovery timeout for this check
	checkCtx, cancel := context.WithTimeout(ctx, protocol.DiscoveryTimeout)
	defer cancel()
	
	if options.Debug {
		fmt.Printf("[DEBUG] hasActiveServer: Checking port %d with %v timeout\n", port, protocol.DiscoveryTimeout)
	}
	
	// Fast path: try a simple TCP connection first to see if anything is listening
	testAddr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", testAddr, protocol.DiscoveryTimeout/2)
	if err == nil {
		conn.Close()
		// Something is listening, now check protocols
		_, err := tryProtocolsOnPort(checkCtx, host, port, options)
		
		if options.Debug {
			if err == nil {
				fmt.Printf("[DEBUG] hasActiveServer: Port %d has active server\n", port)
			} else {
				fmt.Printf("[DEBUG] hasActiveServer: Port %d has listener but no valid protocol\n", port)
			}
		}
		
		return err == nil
	}
	
	// TCP failed, maybe it's UDP only - try protocols directly
	_, err = tryProtocolsOnPort(checkCtx, host, port, options)
	
	if options.Debug {
		if err == nil {
			fmt.Printf("[DEBUG] hasActiveServer: Port %d has active server (UDP)\n", port)
		} else {
			fmt.Printf("[DEBUG] hasActiveServer: Port %d check failed: %v\n", port, err)
		}
	}
	
	return err == nil
}

// determinePortsToScan determines which ports to scan based on options and specified port
func determinePortsToScan(options *protocol.Options, specifiedPort int) []int {
	if len(options.PortRange) > 0 {
		// Use custom port range
		return options.PortRange
	} else if specifiedPort != 0 {
		// Single port specified
		return []int{specifiedPort}
	} else {
		// Scan all default ports
		defaultPorts := make(map[int]bool)
		for _, proto := range protocol.AllProtocols() {
			defaultPorts[proto.DefaultPort()] = true
		}
		var portsToScan []int
		for port := range defaultPorts {
			portsToScan = append(portsToScan, port)
		}
		return portsToScan
	}
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

// setServerInfoFields sets common fields on ServerInfo
func setServerInfoFields(info *protocol.ServerInfo, host string, port int, start time.Time) {
	info.Address = host
	info.Port = port
	// Game field should be set by the protocol implementation, not here
	info.Ping = int(time.Since(start).Nanoseconds() / 1e6)
}

// DefaultOptions returns default query options
func DefaultOptions() *protocol.Options {
	return &protocol.Options{
		Timeout: 5 * time.Second,
		Port:    0, // Use protocol default
		Players: false,
		PortRange: nil,
		MaxConcurrency: 0, // unlimited
		DiscoveryMode: false,
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

// tryProtocolsOnPort tries all protocols on a single port until one succeeds
func tryProtocolsOnPort(ctx context.Context, host string, port int, options *protocol.Options) (*protocol.ServerInfo, error) {
	testAddr := net.JoinHostPort(host, strconv.Itoa(port))
	
	if options.Debug {
		fmt.Printf("[DEBUG] tryProtocolsOnPort: Testing %s with %d protocols\n", testAddr, len(protocol.AllProtocols()))
	}
	
	// Get protocols in order of likelihood for this port
	protocolsToTry := getOrderedProtocols(port)
	
	// Try each protocol until one succeeds
	for _, proto := range protocolsToTry {
		if options.Debug {
			fmt.Printf("[DEBUG] tryProtocolsOnPort: Trying %s protocol on %s\n", proto.Name(), testAddr)
		}
		
		start := time.Now()
		info, err := proto.Query(ctx, testAddr, options)
		
		if err == nil && info.Online {
			if options.Debug {
				fmt.Printf("[DEBUG] tryProtocolsOnPort: SUCCESS with %s protocol (took %v)\n", proto.Name(), time.Since(start))
			}
			setServerInfoFields(info, host, port, start)
			return info, nil
		} else if options.Debug {
			fmt.Printf("[DEBUG] tryProtocolsOnPort: FAILED with %s protocol (took %v): %v\n", proto.Name(), time.Since(start), err)
		}
		
		// Check if main context is cancelled
		select {
		case <-ctx.Done():
			if options.Debug {
				fmt.Printf("[DEBUG] tryProtocolsOnPort: Context cancelled\n")
			}
			return nil, ctx.Err()
		default:
		}
	}
	
	return nil, fmt.Errorf("no responsive server found on port %d", port)
}

// getOrderedProtocols returns protocols in order of likelihood for the given port
func getOrderedProtocols(port int) []protocol.Protocol {
	allProtocols := protocol.AllProtocols()
	ordered := make([]protocol.Protocol, 0, len(allProtocols))
	remaining := make([]protocol.Protocol, 0, len(allProtocols))
	
	// First, try protocols that match this port's default
	for _, proto := range allProtocols {
		if proto.DefaultPort() == port {
			ordered = append(ordered, proto)
		} else {
			remaining = append(remaining, proto)
		}
	}
	
	// Then try remaining protocols
	ordered = append(ordered, remaining...)
	return ordered
}

// queryWithServerInfo handles the common pattern of proto.Query + setServerInfoFields
func queryWithServerInfo(ctx context.Context, proto protocol.Protocol, host string, port int, options *protocol.Options) (*protocol.ServerInfo, error) {
	testAddr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	info, err := proto.Query(ctx, testAddr, options)
	if err != nil {
		return nil, err
	}
	
	if info.Online {
		setServerInfoFields(info, host, port, start)
	}
	
	return info, nil
}

// createDiscoveryOptions standardizes discovery option setup
func createDiscoveryOptions(baseOptions *protocol.Options) *protocol.Options {
	discoveryOptions := *baseOptions
	discoveryOptions.DiscoveryMode = true
	return &discoveryOptions
}


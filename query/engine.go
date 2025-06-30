package query

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/0xkowalskidev/gameserverquery/protocol"
)

// QueryEngine encapsulates all common query logic and provides a unified interface
type QueryEngine struct {
	// Cache for active port discovery to avoid repeated scans
	portCache map[string][]int
	cacheMux  sync.RWMutex
}

// NewQueryEngine creates a new QueryEngine instance
func NewQueryEngine() *QueryEngine {
	return &QueryEngine{
		portCache: make(map[string][]int),
	}
}

// QueryType represents the type of query being performed
type QueryType int

const (
	QueryTypeSingle QueryType = iota
	QueryTypeAutoDetect
	QueryTypeDiscovery
)

// QueryRequest represents a unified query request
type QueryRequest struct {
	Type                QueryType
	Address             string
	Game                string                 // For single protocol queries
	Options             *protocol.Options
	ProgressCallback    func(ScanProgress)     // For discovery queries
}

// QueryResult represents the result of a query operation
type QueryResult struct {
	Servers []*protocol.ServerInfo
	Error   error
}

// PortDiscoveryStrategy defines how ports should be discovered
type PortDiscoveryStrategy interface {
	GetPorts(ctx context.Context, host string, options *protocol.Options) ([]int, error)
}

// ProtocolSelectionStrategy defines how protocols should be selected
type ProtocolSelectionStrategy interface {
	GetProtocols(port int) []protocol.Protocol
}

// SinglePortStrategy discovers ports for a single protocol query
type SinglePortStrategy struct {
	Protocol     protocol.Protocol
	SpecifiedPort int
}

func (s *SinglePortStrategy) GetPorts(ctx context.Context, host string, options *protocol.Options) ([]int, error) {
	// Determine the main port to try
	mainPort := s.SpecifiedPort
	if mainPort == 0 {
		mainPort = s.Protocol.DefaultPort()
	}
	
	ports := []int{mainPort}
	
	// Add adjacent ports for discovery
	const adjacentPortRange = 3
	for offset := 1; offset <= adjacentPortRange; offset++ {
		// Try port + offset
		testPort := mainPort + offset
		if testPort <= 65535 {
			ports = append(ports, testPort)
		}

		// Try port - offset
		testPort = mainPort - offset
		if testPort >= 1024 {
			ports = append(ports, testPort)
		}
	}
	
	return ports, nil
}

// AutoDetectPortStrategy discovers ports for auto-detection
type AutoDetectPortStrategy struct {
	SpecifiedPort int
}

func (s *AutoDetectPortStrategy) GetPorts(ctx context.Context, host string, options *protocol.Options) ([]int, error) {
	if s.SpecifiedPort != 0 {
		return []int{s.SpecifiedPort}, nil
	}
	
	// Get default ports for all protocols
	portMap := make(map[int]bool)
	for _, proto := range protocol.AllProtocols() {
		portMap[proto.DefaultPort()] = true
	}
	
	var ports []int
	for port := range portMap {
		ports = append(ports, port)
	}
	
	return ports, nil
}

// DiscoveryPortStrategy discovers ports for multi-server scanning
type DiscoveryPortStrategy struct {
	SpecifiedPort int
	PortRange     []int
}

func (s *DiscoveryPortStrategy) GetPorts(ctx context.Context, host string, options *protocol.Options) ([]int, error) {
	if len(s.PortRange) > 0 {
		return s.PortRange, nil
	}
	
	if s.SpecifiedPort != 0 {
		return []int{s.SpecifiedPort}, nil
	}
	
	// Use dynamic discovery
	return s.discoverPortsDynamically(ctx, host, options), nil
}

func (s *DiscoveryPortStrategy) discoverPortsDynamically(ctx context.Context, host string, options *protocol.Options) []int {
	const deadPortThreshold = 3
	const minPort = 1024
	const maxPort = 65535

	// Get unique default ports as seeds
	seedPorts := make(map[int]bool)
	for _, proto := range protocol.AllProtocols() {
		seedPorts[proto.DefaultPort()] = true
	}

	allPorts := make(map[int]bool)
	
	// For each unique seed port, expand outward
	for seedPort := range seedPorts {
		// Check the seed port itself
		if s.hasActiveServer(ctx, host, seedPort, options) {
			allPorts[seedPort] = true
		}
		
		// Scan upward from seed
		consecutiveFailures := 0
		for port := seedPort + 1; port <= maxPort; port++ {
			if allPorts[port] {
				consecutiveFailures = 0
				continue
			}
			
			if s.hasActiveServer(ctx, host, port, options) {
				allPorts[port] = true
				consecutiveFailures = 0
			} else {
				consecutiveFailures++
				if consecutiveFailures >= deadPortThreshold {
					break
				}
			}
		}
		
		// Scan downward from seed
		consecutiveFailures = 0
		for port := seedPort - 1; port >= minPort; port-- {
			if allPorts[port] {
				consecutiveFailures = 0
				continue
			}
			
			if s.hasActiveServer(ctx, host, port, options) {
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
	
	// Convert to sorted slice
	var ports []int
	for port := range allPorts {
		ports = append(ports, port)
	}
	
	return ports
}

func (s *DiscoveryPortStrategy) hasActiveServer(ctx context.Context, host string, port int, options *protocol.Options) bool {
	// Use discovery timeout for this check
	checkCtx, cancel := context.WithTimeout(ctx, protocol.DiscoveryTimeout)
	defer cancel()
	
	// Fast path: try a simple TCP connection first
	testAddr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", testAddr, protocol.DiscoveryTimeout/2)
	if err == nil {
		conn.Close()
		// Something is listening, now check protocols
		engine := NewQueryEngine()
		_, err := engine.tryProtocolsOnPort(checkCtx, host, port, options)
		return err == nil
	}
	
	// TCP failed, maybe it's UDP only - try protocols directly  
	engine := NewQueryEngine()
	_, err = engine.tryProtocolsOnPort(checkCtx, host, port, options)
	return err == nil
}

// SingleProtocolStrategy selects a single specific protocol
type SingleProtocolStrategy struct {
	Protocol protocol.Protocol
}

func (s *SingleProtocolStrategy) GetProtocols(port int) []protocol.Protocol {
	return []protocol.Protocol{s.Protocol}
}

// AutoDetectProtocolStrategy selects protocols in order of popularity
type AutoDetectProtocolStrategy struct{}

func (s *AutoDetectProtocolStrategy) GetProtocols(port int) []protocol.Protocol {
	// Get protocols in order of likelihood for this port
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

// AllProtocolsStrategy tries all protocols
type AllProtocolsStrategy struct{}

func (s *AllProtocolsStrategy) GetProtocols(port int) []protocol.Protocol {
	allProtocols := protocol.AllProtocols()
	protocols := make([]protocol.Protocol, 0, len(allProtocols))
	for _, proto := range allProtocols {
		protocols = append(protocols, proto)
	}
	return protocols
}

// Core query methods moved from query.go

// tryProtocolsOnPort tries all protocols on a single port until one succeeds
func (e *QueryEngine) tryProtocolsOnPort(ctx context.Context, host string, port int, options *protocol.Options) (*protocol.ServerInfo, error) {
	testAddr := net.JoinHostPort(host, strconv.Itoa(port))
	
	if options.Debug {
		fmt.Printf("[DEBUG] tryProtocolsOnPort: Testing %s with %d protocols\n", testAddr, len(protocol.AllProtocols()))
	}
	
	// Get protocols in order of likelihood for this port
	strategy := &AutoDetectProtocolStrategy{}
	protocolsToTry := strategy.GetProtocols(port)
	
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
			e.setServerInfoFields(info, host, port, start, proto.Name())
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

// queryWithServerInfo handles the common pattern of proto.Query + setServerInfoFields
func (e *QueryEngine) queryWithServerInfo(ctx context.Context, proto protocol.Protocol, host string, port int, options *protocol.Options) (*protocol.ServerInfo, error) {
	testAddr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	info, err := proto.Query(ctx, testAddr, options)
	if err != nil {
		return nil, err
	}
	
	if info.Online {
		e.setServerInfoFields(info, host, port, start, proto.Name())
	}
	
	return info, nil
}

// setServerInfoFields sets common fields on ServerInfo
func (e *QueryEngine) setServerInfoFields(info *protocol.ServerInfo, host string, port int, start time.Time, protocolName string) {
	info.Address = host
	info.Port = port
	info.Ping = int(time.Since(start).Nanoseconds() / 1e6)
	
	// Game detection is now handled by the protocols themselves
}

// createDiscoveryOptions standardizes discovery option setup
func (e *QueryEngine) createDiscoveryOptions(baseOptions *protocol.Options) *protocol.Options {
	discoveryOptions := *baseOptions
	discoveryOptions.DiscoveryMode = true
	return &discoveryOptions
}

// Execute performs a query based on the request type
func (e *QueryEngine) Execute(ctx context.Context, req *QueryRequest) *QueryResult {
	switch req.Type {
	case QueryTypeSingle:
		return e.executeSingleQuery(ctx, req)
	case QueryTypeAutoDetect:
		return e.executeAutoDetectQuery(ctx, req)
	case QueryTypeDiscovery:
		return e.executeDiscoveryQuery(ctx, req)
	default:
		return &QueryResult{Error: fmt.Errorf("unsupported query type: %v", req.Type)}
	}
}

func (e *QueryEngine) executeSingleQuery(ctx context.Context, req *QueryRequest) *QueryResult {
	proto, exists := protocol.GetProtocol(req.Game)
	if !exists {
		return &QueryResult{Error: fmt.Errorf("unsupported game: %s", req.Game)}
	}

	// Parse address and determine port
	host, port, err := parseAddress(req.Address, req.Options.Port, proto.DefaultPort())
	if err != nil {
		return &QueryResult{Error: fmt.Errorf("invalid address: %w", err)}
	}

	// Create port discovery strategy
	strategy := &SinglePortStrategy{
		Protocol:      proto,
		SpecifiedPort: port,
	}
	
	ports, err := strategy.GetPorts(ctx, host, req.Options)
	if err != nil {
		return &QueryResult{Error: err}
	}

	// Try the specified port first
	info, err := e.queryWithServerInfo(ctx, proto, host, ports[0], req.Options)
	if err == nil && info.Online {
		return &QueryResult{Servers: []*protocol.ServerInfo{info}}
	}

	// If that failed, try adjacent ports
	discoveryOptions := e.createDiscoveryOptions(req.Options)
	
	for _, testPort := range ports[1:] {
		if info, err := e.tryProtocolsOnPort(ctx, host, testPort, discoveryOptions); err == nil {
			return &QueryResult{Servers: []*protocol.ServerInfo{info}}
		}
	}

	return &QueryResult{Error: fmt.Errorf("no responsive server found at %s or adjacent ports", req.Address)}
}

func (e *QueryEngine) executeAutoDetectQuery(ctx context.Context, req *QueryRequest) *QueryResult {
	host, port, err := parseAddress(req.Address, req.Options.Port, 0)
	if err != nil {
		return &QueryResult{Error: fmt.Errorf("invalid address: %w", err)}
	}

	// If port is specified, try to match it to a known default port first
	if port != 0 {
		// Get protocols ordered by likelihood for this specific port
		protocolsForPort := e.getProtocolsByPortPreference(port)
		
		for _, proto := range protocolsForPort {
			info, err := e.queryWithServerInfo(ctx, proto, host, port, req.Options)
			if err == nil && info.Online {
				return &QueryResult{Servers: []*protocol.ServerInfo{info}}
			}
		}
	}

	// Try all protocols on their default ports, ordered by popularity
	popularityOrder := e.getProtocolsByPopularity()
	
	for _, proto := range popularityOrder {
		testPort := port
		if testPort == 0 {
			testPort = proto.DefaultPort()
		}
		
		info, err := e.queryWithServerInfo(ctx, proto, host, testPort, req.Options)
		if err == nil && info.Online {
			return &QueryResult{Servers: []*protocol.ServerInfo{info}}
		}
	}

	return &QueryResult{Error: fmt.Errorf("no responsive server found at %s", req.Address)}
}

// getProtocolsByPortPreference returns protocols ordered by likelihood for a specific port
func (e *QueryEngine) getProtocolsByPortPreference(port int) []protocol.Protocol {
	allProtocols := protocol.AllProtocols()
	var matching, remaining []protocol.Protocol
	
	// First try protocols that use this port as default
	for _, proto := range allProtocols {
		if proto.DefaultPort() == port {
			matching = append(matching, proto)
		} else {
			remaining = append(remaining, proto)
		}
	}
	
	// Then try remaining protocols
	return append(matching, remaining...)
}

// getProtocolsByPopularity returns protocols ordered by general popularity/likelihood
func (e *QueryEngine) getProtocolsByPopularity() []protocol.Protocol {
	// Ordered by general popularity and likelihood of being found
	popularityOrder := []string{
		"minecraft",    // Very common
		"source",       // Covers many Steam games
		"terraria",     // Popular indie game
		"rust",         // Popular but uses source protocol
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
	for name, proto := range protocol.AllProtocols() {
		if !used[name] {
			result = append(result, proto)
		}
	}
	
	return result
}

func (e *QueryEngine) executeDiscoveryQuery(ctx context.Context, req *QueryRequest) *QueryResult {
	host, specifiedPort, err := parseAddress(req.Address, req.Options.Port, 0)
	if err != nil {
		return &QueryResult{Error: fmt.Errorf("invalid address: %w", err)}
	}

	// Create port discovery strategy
	strategy := &DiscoveryPortStrategy{
		SpecifiedPort: specifiedPort,
		PortRange:     req.Options.PortRange,
	}
	
	portsToScan, err := strategy.GetPorts(ctx, host, req.Options)
	if err != nil {
		return &QueryResult{Error: err}
	}

	// Set up concurrency control
	maxConcurrency := req.Options.MaxConcurrency
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
	if req.ProgressCallback != nil {
		req.ProgressCallback(ScanProgress{
			TotalPorts:     len(portsToScan),
			TotalProtocols: totalProtocols,
			Completed:      0,
			ServersFound:   0,
		})
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
					info, err := proto.Query(ctx, testAddr, req.Options)
					
					// Update progress
					progressMux.Lock()
					completed++
					if err == nil && info.Online {
						serversFound++
						e.setServerInfoFields(info, host, port, start, proto.Name())
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
					if req.ProgressCallback != nil {
						req.ProgressCallback(currentProgress)
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

	return &QueryResult{Servers: servers}
}
package query

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
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

// Simplified port and protocol discovery functions replace strategy patterns

// getSingleProtocolPorts returns ports to try for a single protocol query
func getSingleProtocolPorts(proto protocol.Protocol, specifiedPort int) []int {
	// Determine the main port to try
	mainPort := specifiedPort
	if mainPort == 0 {
		mainPort = proto.DefaultPort()
	}
	
	ports := []int{mainPort}
	
	// Add adjacent ports for discovery
	const adjacentPortRange = 3
	for offset := 1; offset <= adjacentPortRange; offset++ {
		// Try port + offset
		if testPort := mainPort + offset; testPort <= 65535 {
			ports = append(ports, testPort)
		}
		// Try port - offset
		if testPort := mainPort - offset; testPort >= 1024 {
			ports = append(ports, testPort)
		}
	}
	
	return ports
}

// getDiscoveryPorts returns common game server ports for discovery
func getDiscoveryPorts(ctx context.Context, host string, options *protocol.Options) []int {
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
	
	if options.Debug {
		debugLogf("Discovery", "Using %d common game ports for discovery", len(ports))
	}
	
	return ports
}

// getProtocolsForPort returns protocols ordered by likelihood for the given port
func getProtocolsForPort(port int) []protocol.Protocol {
	allProtocols := protocol.AllProtocols()
	ordered := make([]protocol.Protocol, 0, len(allProtocols))
	remaining := make([]protocol.Protocol, 0, len(allProtocols))
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
	ordered = append(ordered, remaining...)
	return ordered
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
	
	// Get unique ports from all game configurations
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

	if options.Debug {
		debugLogf("Discovery", "Starting dynamic port discovery for %s", host)
		debugLogf("Discovery", "Port range %d-%d, dead port threshold %d", minPort, maxPort, deadPortThreshold)
	}

	// Get unique default query ports as seeds (prioritize query ports for discovery)
	seedPorts := make(map[int]bool)
	for _, proto := range protocol.AllProtocols() {
		seedPorts[proto.DefaultQueryPort()] = true
		// Also include game ports for comprehensive discovery
		seedPorts[proto.DefaultPort()] = true
	}

	if options.Debug {
		debugLogf("Discovery", "Found %d unique seed ports from protocols", len(seedPorts))
		seedList := make([]int, 0, len(seedPorts))
		for port := range seedPorts {
			seedList = append(seedList, port)
		}
		debugLogf("Discovery", "Seed ports: %v", seedList)
	}

	allPorts := make(map[int]bool)
	
	// For each unique seed port, expand outward
	for seedPort := range seedPorts {
		if options.Debug {
			debugLogf("Discovery", "Checking seed port %d", seedPort)
		}
		
		// Check the seed port itself
		if s.hasActiveServer(ctx, host, seedPort, options) {
			allPorts[seedPort] = true
			if options.Debug {
				debugLogf("Discovery", "Seed port %d has active server", seedPort)
			}
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
	
	if options.Debug {
		debugLogf("Discovery", "Discovered %d active ports: %v", len(ports), ports)
	}
	
	return ports
}

func (s *DiscoveryPortStrategy) hasActiveServer(ctx context.Context, host string, port int, options *protocol.Options) bool {
	// Use discovery timeout for this check
	checkCtx, cancel := context.WithTimeout(ctx, protocol.DiscoveryTimeout)
	defer cancel()
	
	start := time.Now()
	testAddr := net.JoinHostPort(host, strconv.Itoa(port))
	
	if options.Debug {
		debugLogf("Discovery", "Checking %s for active server", testAddr)
	}
	
	// Fast path: try a simple TCP connection first
	conn, err := net.DialTimeout("tcp", testAddr, protocol.DiscoveryTimeout/2)
	if err == nil {
		conn.Close()
		if options.Debug {
			debugLogf("Discovery", "TCP connection successful to %s, checking protocols", testAddr)
		}
		// Something is listening, now check protocols
		engine := NewQueryEngine()
		_, err := engine.tryProtocolsOnPort(checkCtx, host, port, port, options)
		result := err == nil
		if options.Debug {
			debugLogf("Discovery", "Protocol check on %s: %v (took %v)", testAddr, result, time.Since(start))
		}
		return result
	}
	
	if options.Debug {
		debugLogf("Discovery", "TCP connection failed to %s, trying UDP protocols", testAddr)
	}
	
	// TCP failed, maybe it's UDP only - try protocols directly  
	engine := NewQueryEngine()
	_, err = engine.tryProtocolsOnPort(checkCtx, host, port, port, options)
	result := err == nil
	if options.Debug {
		debugLogf("Discovery", "UDP protocol check on %s: %v (took %v)", testAddr, result, time.Since(start))
	}
	return result
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
func (e *QueryEngine) tryProtocolsOnPort(ctx context.Context, host string, requestedPort int, queryPort int, options *protocol.Options) (*protocol.ServerInfo, error) {
	testAddr := net.JoinHostPort(host, strconv.Itoa(queryPort))
	
	if options.Debug {
		debugLogf("Engine", "Testing %s with %d protocols", testAddr, len(protocol.AllProtocols()))
	}
	
	// Get protocols in order of likelihood for this port
	protocolsToTry := getProtocolsForPort(queryPort)
	
	// Try each protocol until one succeeds
	for _, proto := range protocolsToTry {
		if options.Debug {
			debugLogf("Engine", "Trying %s protocol on %s", proto.Name(), testAddr)
		}
		
		start := time.Now()
		info, err := proto.Query(ctx, testAddr, options)
		
		if err == nil && info.Online {
			if options.Debug {
				debugLogf("Engine", "SUCCESS with %s protocol (took %v)", proto.Name(), time.Since(start))
			}
			e.setServerInfoFields(info, host, requestedPort, queryPort, start, proto.Name())
			return info, nil
		} else if options.Debug {
			debugLogf("Engine", "FAILED with %s protocol (took %v): %v", proto.Name(), time.Since(start), err)
		}
		
		// Check if main context is cancelled
		select {
		case <-ctx.Done():
			if options.Debug {
				debugLog("Engine", "Context cancelled")
			}
			return nil, ctx.Err()
		default:
		}
	}
	
	return nil, fmt.Errorf("no responsive server found on port %d", queryPort)
}

// queryWithServerInfo handles the common pattern of proto.Query + setServerInfoFields
func (e *QueryEngine) queryWithServerInfo(ctx context.Context, proto protocol.Protocol, host string, requestedPort int, queryPort int, options *protocol.Options) (*protocol.ServerInfo, error) {
	testAddr := net.JoinHostPort(host, strconv.Itoa(queryPort))
	start := time.Now()
	
	if options.Debug {
		debugLogf("Engine", "Querying %s with %s protocol", testAddr, proto.Name())
	}
	
	info, err := proto.Query(ctx, testAddr, options)
	elapsed := time.Since(start)
	
	if err != nil {
		if options.Debug {
			debugLogf("Engine", "Query failed for %s (%s): %v (took %v)", testAddr, proto.Name(), err, elapsed)
		}
		return nil, err
	}
	
	if info.Online {
		e.setServerInfoFields(info, host, requestedPort, queryPort, start, proto.Name())
		if options.Debug {
			debugLogf("Engine", "Query successful for %s (%s): online=%v, players=%d/%d (took %v)", 
				testAddr, proto.Name(), info.Online, info.Players.Current, info.Players.Max, elapsed)
		}
	} else {
		if options.Debug {
			debugLogf("Engine", "Server %s (%s) is offline (took %v)", testAddr, proto.Name(), elapsed)
		}
	}
	
	return info, nil
}

// setServerInfoFields sets common fields on ServerInfo
func (e *QueryEngine) setServerInfoFields(info *protocol.ServerInfo, host string, requestedPort int, queryPort int, start time.Time, protocolName string) {
	info.Address = host
	info.Port = requestedPort
	info.QueryPort = queryPort
	
	// Only set ping if the protocol didn't provide one (ping == 0)
	if info.Ping == 0 {
		info.Ping = int(math.Ceil(float64(time.Since(start).Nanoseconds()) / 1e6))
	}
	
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
	if req.Options.Debug {
		debugLogf("Query", "Starting single query for game '%s' at address '%s'", req.Game, req.Address)
	}
	
	// Get game config and protocol
	gameConfig, proto, exists := protocol.GetGameConfigFromRegistry(req.Game)
	if !exists {
		if req.Options.Debug {
			debugLogf("Query", "Unsupported game: %s", req.Game)
		}
		return &QueryResult{Error: fmt.Errorf("unsupported game: %s", req.Game)}
	}

	// Parse address and determine port - use game's query port by default
	host, requestedPort, err := parseAddress(req.Address, req.Options.Port, gameConfig.QueryPort)
	if err != nil {
		if req.Options.Debug {
			debugLogf("Query", "Address parsing failed: %v", err)
		}
		return &QueryResult{Error: fmt.Errorf("invalid address: %w", err)}
	}
	
	if req.Options.Debug {
		debugLogf("Query", "Parsed address - host: %s, requested port: %d, protocol: %s", host, requestedPort, proto.Name())
	}

	// Get ports to try for single protocol query
	ports := getSingleProtocolPorts(proto, requestedPort)

	// Try the specified port first with shorter timeout since we have adjacent ports as backup
	if req.Options.Debug {
		debugLogf("Query", "Trying primary port %d with %s protocol", ports[0], proto.Name())
	}
	
	// Use normal timeout for primary query since it's the exact port requested
	info, err := e.queryWithServerInfo(ctx, proto, host, requestedPort, ports[0], req.Options)
	if err == nil && info.Online {
		if req.Options.Debug {
			debugLogf("Query", "SUCCESS on primary port %d", ports[0])
		}
		return &QueryResult{Servers: []*protocol.ServerInfo{info}}
	}
	
	if req.Options.Debug {
		debugLogf("Query", "Primary port %d failed: %v", ports[0], err)
		debugLogf("Query", "Trying %d adjacent ports with protocol detection", len(ports)-1)
	}

	// If that failed, try adjacent ports with fresh context
	discoveryOptions := e.createDiscoveryOptions(req.Options)
	
	// Create fresh context for adjacent port discovery with discovery timeout
	discoveryCtx, cancel := context.WithTimeout(context.Background(), protocol.DiscoveryTimeout*time.Duration(len(ports[1:])*4))
	defer cancel()
	
	for i, testPort := range ports[1:] {
		if req.Options.Debug {
			debugLogf("Query", "Trying adjacent port %d (%d/%d)", testPort, i+1, len(ports)-1)
		}
		if info, err := e.tryProtocolsOnPort(discoveryCtx, host, requestedPort, testPort, discoveryOptions); err == nil {
			if req.Options.Debug {
				debugLogf("Query", "SUCCESS on adjacent port %d", testPort)
			}
			return &QueryResult{Servers: []*protocol.ServerInfo{info}}
		}
	}

	if req.Options.Debug {
		debugLog("Query", "All ports failed, no responsive server found")
	}
	return &QueryResult{Error: fmt.Errorf("no responsive server found at %s or adjacent ports", req.Address)}
}

func (e *QueryEngine) executeAutoDetectQuery(ctx context.Context, req *QueryRequest) *QueryResult {
	if req.Options.Debug {
		debugLogf("AutoDetect", "Starting auto-detection for address '%s'", req.Address)
	}
	
	host, port, err := parseAddress(req.Address, req.Options.Port, 0)
	if err != nil {
		if req.Options.Debug {
			debugLogf("AutoDetect", "Address parsing failed: %v", err)
		}
		return &QueryResult{Error: fmt.Errorf("invalid address: %w", err)}
	}
	
	if req.Options.Debug {
		debugLogf("AutoDetect", "Parsed address - host: %s, port: %d", host, port)
	}

	// If port is specified, try to match it to a known default port first
	if port != 0 {
		// Get protocols ordered by likelihood for this specific port
		protocolsForPort := e.getProtocolsByPortPreference(port)
		
		if req.Options.Debug {
			debugLogf("AutoDetect", "Port %d specified, trying %d matching protocols first", port, len(protocolsForPort))
		}
		
		for i, proto := range protocolsForPort {
			if req.Options.Debug {
				debugLogf("AutoDetect", "Trying protocol %s on port %d (%d/%d)", proto.Name(), port, i+1, len(protocolsForPort))
			}
			
			// Use shorter timeout for auto-detection since we have adjacent ports as backup
			quickCtx, quickCancel := context.WithTimeout(ctx, protocol.DiscoveryTimeout*3)
			quickOptions := e.createDiscoveryOptions(req.Options)
			
			info, err := e.queryWithServerInfo(quickCtx, proto, host, port, port, quickOptions)
			quickCancel()
			
			if err == nil && info.Online {
				if req.Options.Debug {
					debugLogf("AutoDetect", "SUCCESS with %s on port %d", proto.Name(), port)
				}
				return &QueryResult{Servers: []*protocol.ServerInfo{info}}
			}
			if req.Options.Debug {
				debugLogf("AutoDetect", "FAILED with %s on port %d: %v", proto.Name(), port, err)
			}
		}
	}

	// If port was specified but all protocols failed, try adjacent ports with protocol detection
	if port != 0 {
		if req.Options.Debug {
			debugLogf("AutoDetect", "Specified port %d failed, trying adjacent ports with protocol detection", port)
		}
		
		// Create fresh context for adjacent port discovery with discovery timeout
		const adjacentPortRange = 3
		estimatedTime := protocol.DiscoveryTimeout * time.Duration(adjacentPortRange*2*4) // ports * directions * protocols
		discoveryCtx, cancel := context.WithTimeout(context.Background(), estimatedTime)
		defer cancel()
		
		discoveryOptions := e.createDiscoveryOptions(req.Options)
		
		// Try adjacent ports (±3 range like SinglePortStrategy)
		for offset := 1; offset <= adjacentPortRange; offset++ {
			// Try port + offset
			testPort := port + offset
			if testPort <= 65535 {
				if req.Options.Debug {
					debugLogf("AutoDetect", "Trying adjacent port %d (+%d)", testPort, offset)
				}
				if info, err := e.tryProtocolsOnPort(discoveryCtx, host, port, testPort, discoveryOptions); err == nil {
					if req.Options.Debug {
						debugLogf("AutoDetect", "SUCCESS on adjacent port %d", testPort)
					}
					return &QueryResult{Servers: []*protocol.ServerInfo{info}}
				}
			}

			// Try port - offset
			testPort = port - offset
			if testPort >= 1024 {
				if req.Options.Debug {
					debugLogf("AutoDetect", "Trying adjacent port %d (-%d)", testPort, offset)
				}
				if info, err := e.tryProtocolsOnPort(discoveryCtx, host, port, testPort, discoveryOptions); err == nil {
					if req.Options.Debug {
						debugLogf("AutoDetect", "SUCCESS on adjacent port %d", testPort)
					}
					return &QueryResult{Servers: []*protocol.ServerInfo{info}}
				}
			}
		}
	}

	// Try all protocols on their default ports, ordered by popularity
	popularityOrder := e.getProtocolsByPopularity()
	
	if req.Options.Debug {
		debugLogf("AutoDetect", "Trying %d protocols on their default ports", len(popularityOrder))
	}
	
	for i, proto := range popularityOrder {
		testPort := port
		if testPort == 0 {
			testPort = proto.DefaultQueryPort()
		}
		
		if req.Options.Debug {
			debugLogf("AutoDetect", "Trying protocol %s on default port %d (%d/%d)", proto.Name(), testPort, i+1, len(popularityOrder))
		}
		
		info, err := e.queryWithServerInfo(ctx, proto, host, port, testPort, req.Options)
		if err == nil && info.Online {
			if req.Options.Debug {
				debugLogf("AutoDetect", "SUCCESS with %s on default port %d", proto.Name(), testPort)
			}
			return &QueryResult{Servers: []*protocol.ServerInfo{info}}
		}
		if req.Options.Debug {
			debugLogf("AutoDetect", "FAILED with %s on default port %d: %v", proto.Name(), testPort, err)
		}
	}

	if req.Options.Debug {
		debugLog("AutoDetect", "All protocols failed, no responsive server found")
	}
	return &QueryResult{Error: fmt.Errorf("no responsive server found at %s", req.Address)}
}

// getProtocolsByPortPreference returns protocols ordered by likelihood for a specific port
func (e *QueryEngine) getProtocolsByPortPreference(port int) []protocol.Protocol {
	allProtocols := protocol.AllProtocols()
	var matching, remaining []protocol.Protocol
	
	// First try protocols that use this port as default query port, then game port
	for _, proto := range allProtocols {
		if proto.DefaultQueryPort() == port {
			matching = append(matching, proto)
		} else if proto.DefaultPort() == port {
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
		"a2s",          // Covers many Steam games
		"terraria",     // Popular indie game
		"rust",         // Popular but uses a2s protocol
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
	if req.Options.Debug {
		debugLogf("Discovery", "Starting server discovery for address '%s'", req.Address)
	}
	
	host, specifiedPort, err := parseAddress(req.Address, req.Options.Port, 0)
	if err != nil {
		if req.Options.Debug {
			debugLogf("Discovery", "Address parsing failed: %v", err)
		}
		return &QueryResult{Error: fmt.Errorf("invalid address: %w", err)}
	}
	
	if req.Options.Debug {
		debugLogf("Discovery", "Parsed address - host: %s, port: %d", host, specifiedPort)
	}

	// Get ports to scan for discovery
	var portsToScan []int
	if len(req.Options.PortRange) > 0 {
		// Use custom port range
		portsToScan = req.Options.PortRange
		if req.Options.Debug {
			debugLogf("Discovery", "Using custom port range: %v", req.Options.PortRange)
		}
	} else if specifiedPort != 0 {
		// Use specified port
		portsToScan = []int{specifiedPort}
		if req.Options.Debug {
			debugLogf("Discovery", "Using specified port: %d", specifiedPort)
		}
	} else {
		// Use dynamic discovery
		portsToScan = getDiscoveryPorts(ctx, host, req.Options)
		if req.Options.Debug {
			debugLog("Discovery", "Using dynamic port discovery")
		}
	}
	
	if req.Options.Debug {
		debugLogf("Discovery", "Will scan %d ports: %v", len(portsToScan), portsToScan)
	}

	// Set up concurrency control
	maxConcurrency := req.Options.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = len(portsToScan) * len(protocol.AllProtocols())
	}
	semaphore := make(chan struct{}, maxConcurrency)
	
	if req.Options.Debug {
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
						e.setServerInfoFields(info, host, port, port, start, proto.Name())
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

	if req.Options.Debug {
		debugLogf("Discovery", "Discovery complete, found %d servers", len(servers))
	}

	return &QueryResult{Servers: servers}
}

// Debug logging helpers for query package
func debugLog(component, message string) {
	fmt.Fprintf(os.Stderr, "[DEBUG %s] %s: %s\n", time.Now().Format("15:04:05.000"), component, message)
}

func debugLogf(component, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	debugLog(component, message)
}
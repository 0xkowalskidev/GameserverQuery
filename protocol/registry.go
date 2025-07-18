package protocol

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"
)

// GameConfig represents configuration for a specific game that uses this protocol
type GameConfig struct {
	Name      string // Game identifier (e.g., "rust", "cs2", "ark-survival-evolved")
	GamePort  int    // Default port where players connect
	QueryPort int    // Default port for status queries
}

// Protocol defines how to query a specific game server type
type Protocol interface {
	// Query attempts to get server information
	Query(ctx context.Context, addr string, opts *Options) (*ServerInfo, error)

	// Name returns the protocol name (e.g., "minecraft", "a2s")
	Name() string

	// DefaultPort returns the default port for this protocol (where players connect)
	DefaultPort() int

	// DefaultQueryPort returns the default port for status queries
	DefaultQueryPort() int

	// Games returns all games supported by this protocol with their configurations
	Games() []GameConfig
	
	// DetectGame analyzes server response data to determine the specific game
	DetectGame(info *ServerInfo) string
}

// ServerInfo represents information about a game server
type ServerInfo struct {
	Name      string            `json:"name"`
	Game      string            `json:"game"`
	Version   string            `json:"version"`
	Address   string            `json:"address"`
	Port      int               `json:"port"`
	QueryPort int               `json:"query_port"`
	Players   PlayerInfo        `json:"players"`
	Map       string            `json:"map,omitempty"`
	Ping      int               `json:"ping"`
	Online    bool              `json:"online"`
	Extra     map[string]string `json:"extra,omitempty"`
}

// PlayerInfo represents player count and list information
type PlayerInfo struct {
	Current int      `json:"current"`
	Max     int      `json:"max"`
	List    []Player `json:"list,omitempty"`
}

// Player represents an individual player
type Player struct {
	Name     string        `json:"name"`
	Score    int           `json:"score,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
}

// Options configures how queries are performed
type Options struct {
	Timeout time.Duration
	Port    int
	Players bool
	// Discovery options
	PortRange      []int // Custom ports to scan
	MaxConcurrency int   // Maximum concurrent queries (0 = unlimited)
	DiscoveryMode  bool  // Whether this is a discovery scan (uses shorter timeouts)
	Debug          bool  // Enable debug logging
}

// Registry manages protocol registration
type Registry struct {
	protocols map[string]Protocol
	aliases   map[string]string // maps alias to primary protocol name
}

var registry = &Registry{
	protocols: make(map[string]Protocol),
	aliases:   make(map[string]string),
}

// Register adds a protocol to the global registry
func (r *Registry) Register(protocol Protocol) {
	r.protocols[protocol.Name()] = protocol
	
	// Auto-register game names as aliases
	for _, game := range protocol.Games() {
		if game.Name != "" && game.Name != protocol.Name() {
			r.aliases[game.Name] = protocol.Name()
		}
	}
}

// RegisterAlias adds an alias for an existing protocol
func (r *Registry) RegisterAlias(alias, protocolName string) {
	r.aliases[alias] = protocolName
}

// Get retrieves a protocol by name (including aliases)
func (r *Registry) Get(name string) (Protocol, bool) {
	// Check if it's a direct protocol name
	if protocol, exists := r.protocols[name]; exists {
		return protocol, true
	}

	// Check if it's an alias
	if protocolName, exists := r.aliases[name]; exists {
		return r.protocols[protocolName], true
	}

	return nil, false
}

// GetGameConfig retrieves the game configuration for a specific game name
func (r *Registry) GetGameConfig(gameName string) (*GameConfig, Protocol, bool) {
	// Get the protocol (handles aliases)
	protocol, exists := r.Get(gameName)
	if !exists {
		return nil, nil, false
	}
	
	// Find the specific game config
	for _, game := range protocol.Games() {
		if game.Name == gameName {
			return &game, protocol, true
		}
	}
	
	// If no specific game config found, return default
	defaultConfig := &GameConfig{
		Name:      protocol.Name(),
		GamePort:  protocol.DefaultPort(),
		QueryPort: protocol.DefaultQueryPort(),
	}
	return defaultConfig, protocol, true
}

// All returns all registered protocols
func (r *Registry) All() map[string]Protocol {
	result := make(map[string]Protocol)
	for name, protocol := range r.protocols {
		result[name] = protocol
	}
	return result
}

// AllNames returns all protocol names including aliases
func (r *Registry) AllNames() []string {
	names := make([]string, 0, len(r.protocols)+len(r.aliases))

	// Add primary protocol names
	for name := range r.protocols {
		names = append(names, name)
	}

	// Add aliases
	for alias := range r.aliases {
		names = append(names, alias)
	}

	return names
}

// GetProtocol retrieves a protocol by name from the global registry
func GetProtocol(name string) (Protocol, bool) {
	return registry.Get(name)
}

// GetGameConfigFromRegistry returns game configuration from the global registry
func GetGameConfigFromRegistry(gameName string) (*GameConfig, Protocol, bool) {
	return registry.GetGameConfig(gameName)
}

// AllProtocols returns all registered protocols from the global registry
func AllProtocols() map[string]Protocol {
	return registry.All()
}

// AllGameNames returns all game names including aliases
func AllGameNames() []string {
	return registry.AllNames()
}

// RegisterAlias adds an alias for an existing protocol
func RegisterAlias(alias, protocolName string) {
	registry.RegisterAlias(alias, protocolName)
}

// Constants for discovery mode
const DiscoveryTimeout = 300 * time.Millisecond

// getTimeout returns the appropriate timeout based on discovery mode
func getTimeout(opts *Options) time.Duration {
	if opts.DiscoveryMode {
		return DiscoveryTimeout
	}
	return opts.Timeout
}

// setupConnection handles common connection setup with discovery mode timeout
func setupConnection(ctx context.Context, network, addr string, opts *Options) (net.Conn, error) {
	timeout := getTimeout(opts)

	if opts.Debug {
		debugLogf("Connection", "Connecting to %s://%s with timeout %v (discovery mode: %v)",
			network, addr, timeout, opts.DiscoveryMode)
	}

	start := time.Now()
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, network, addr)
	elapsed := time.Since(start)

	if err != nil {
		if opts.Debug {
			debugLogf("Connection", "Connection to %s://%s FAILED: %v (took %v)", network, addr, err, elapsed)
		}
		return nil, fmt.Errorf("connection failed: %w", err)
	}

	if opts.Debug {
		debugLogf("Connection", "Connection to %s://%s successful (took %v)", network, addr, elapsed)
	}

	// Set deadline based on context or timeout
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	conn.SetDeadline(deadline)

	if opts.Debug {
		debugLogf("Connection", "Set deadline for %s://%s to %v", network, addr, deadline)
	}

	return conn, nil
}

// Debug logging helpers
func debugLog(component, message string) {
	fmt.Fprintf(os.Stderr, "[DEBUG %s] %s: %s\n", time.Now().Format("15:04:05.000"), component, message)
}

func debugLogf(component, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	debugLog(component, message)
}

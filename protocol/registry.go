package protocol

import (
	"context"
	"time"
)

// Protocol defines how to query a specific game server type
type Protocol interface {
	// Query attempts to get server information
	Query(ctx context.Context, addr string, opts *Options) (*ServerInfo, error)
	
	// Name returns the protocol name (e.g., "minecraft", "source")
	Name() string
	
	// DefaultPort returns the default port for this protocol
	DefaultPort() int
}

// ServerInfo represents information about a game server
type ServerInfo struct {
	Name        string            `json:"name"`
	Game        string            `json:"game"`
	Version     string            `json:"version"`
	Address     string            `json:"address"`
	Port        int               `json:"port"`
	Players     PlayerInfo        `json:"players"`
	Map         string            `json:"map,omitempty"`
	MOTD        string            `json:"motd,omitempty"`
	Ping        int               `json:"ping"`
	Online      bool              `json:"online"`
	Extra       map[string]string `json:"extra,omitempty"`
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
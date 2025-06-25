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

	proto, exists := protocol.GetProtocol(game)
	if !exists {
		return nil, fmt.Errorf("unsupported game: %s", game)
	}

	// Parse address and determine port
	host, port, err := parseAddress(addr, options.Port, proto.DefaultPort())
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	fullAddr := net.JoinHostPort(host, strconv.Itoa(port))
	
	start := time.Now()
	info, err := proto.Query(ctx, fullAddr, options)
	if err != nil {
		return nil, err
	}

	// Set common fields
	setServerInfoFields(info, host, port, game, start)
	return info, nil
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
	games := []string{"minecraft", "source", "terraria", "valheim", "rust", "ark-survival-evolved", "factorio", "7-days-to-die", "project-zomboid", "satisfactory"}
	
	for _, game := range games {
		if proto, exists := protocol.GetProtocol(game); exists {
			testPort := port
			if testPort == 0 {
				testPort = proto.DefaultPort()
			}
			
			testAddr := net.JoinHostPort(host, strconv.Itoa(testPort))
			start := time.Now()
			info, err := proto.Query(ctx, testAddr, options)
			if err == nil && info.Online {
				setServerInfoFields(info, host, testPort, proto.Name(), start)
				return info, nil
			}
		}
	}

	return nil, fmt.Errorf("no responsive server found at %s", addr)
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
func setServerInfoFields(info *protocol.ServerInfo, host string, port int, game string, start time.Time) {
	info.Address = host
	info.Port = port
	info.Game = game
	info.Ping = int(time.Since(start).Nanoseconds() / 1e6)
}

// DefaultOptions returns default query options
func DefaultOptions() *protocol.Options {
	return &protocol.Options{
		Timeout: 5 * time.Second,
		Port:    0, // Use protocol default
		Players: false,
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


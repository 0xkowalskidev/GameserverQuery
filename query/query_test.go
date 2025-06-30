package query

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSupportedGames(t *testing.T) {
	games := SupportedGames()

	expectedProtocols := []string{"minecraft", "source", "terraria"}
	expectedAliases := []string{"counter-strike-2", "counter-strike", "garrys-mod", "team-fortress-2", "rust", "ark-survival-evolved", "valheim", "7-days-to-die"}
	
	if len(games) < len(expectedProtocols)+len(expectedAliases) {
		t.Errorf("Expected at least %d games, got %d", len(expectedProtocols)+len(expectedAliases), len(games))
	}

	gameMap := make(map[string]bool)
	for _, game := range games {
		gameMap[game] = true
	}

	// Check primary protocols
	for _, expected := range expectedProtocols {
		if !gameMap[expected] {
			t.Errorf("Expected protocol '%s' not found in supported games", expected)
		}
	}
	
	// Check some important aliases
	for _, expected := range expectedAliases {
		if !gameMap[expected] {
			t.Errorf("Expected alias '%s' not found in supported games", expected)
		}
	}
}

func TestDefaultPort(t *testing.T) {
	tests := []struct {
		game         string
		expectedPort int
	}{
		{"minecraft", 25565},
		{"source", 27015},
		{"terraria", 7777},
		{"counter-strike-2", 27015},    // Alias should work
		{"counter-strike", 27015},     // Alias should work
		{"garrys-mod", 27015},         // Alias should work
		{"valheim", 27015},           // Alias should work
		{"7-days-to-die", 27015},        // Alias should work
		{"invalid", 0},
	}

	for _, test := range tests {
		port := DefaultPort(test.game)
		if port != test.expectedPort {
			t.Errorf("DefaultPort(%s): expected %d, got %d", test.game, test.expectedPort, port)
		}
	}
}

func TestGameAliases(t *testing.T) {
	ctx := context.Background()
	
	// Test that aliases resolve to the same protocol
	aliasTests := []struct {
		alias    string
		expected string
	}{
		{"counter-strike-2", "source"},
		{"counter-strike", "source"},
		{"garrys-mod", "source"},
		{"team-fortress-2", "source"},
		{"counter-source", "source"},
		{"rust", "source"},
		{"valheim", "source"},
		{"7-days-to-die", "source"},
	}
	
	for _, test := range aliasTests {
		_, err := Query(ctx, test.alias, "192.168.1.99:25565", Timeout(1*time.Second))
		// We expect an error since server is offline, but it should be a connection error, not "unsupported game"
		if err != nil && strings.Contains(err.Error(), "unsupported game") {
			t.Errorf("Alias %s should be supported but got unsupported game error", test.alias)
		}
	}
}

func TestQueryWithInvalidGame(t *testing.T) {
	ctx := context.Background()

	_, err := Query(ctx, "invalidgame", "localhost:25565", Timeout(5*time.Second))
	if err == nil {
		t.Error("Expected error for invalid game")
	}
}

func TestQueryWithOfflineServer(t *testing.T) {
	ctx := context.Background()

	info, err := Query(ctx, "minecraft", "192.168.1.99:25565", Timeout(2*time.Second))
	if err == nil {
		t.Error("Expected error for offline server")
	}

	if info != nil && info.Online {
		t.Error("Server should be detected as offline")
	}
}

func TestAutoDetectWithOfflineServer(t *testing.T) {
	ctx := context.Background()

	_, err := AutoDetect(ctx, "192.168.1.99:25565", Timeout(2*time.Second))
	if err == nil {
		t.Error("Expected error for offline server in auto-detect")
	}
}

func TestParseAddress(t *testing.T) {
	tests := []struct {
		addr        string
		optPort     int
		defaultPort int
		expectHost  string
		expectPort  int
		expectError bool
	}{
		{"localhost", 0, 25565, "localhost", 25565, false},
		{"localhost:8080", 0, 25565, "localhost", 8080, false},
		{"192.168.1.1:27015", 9999, 25565, "192.168.1.1", 27015, false},
		{"example.com", 9999, 25565, "example.com", 9999, false},
		{"", 0, 25565, "", 0, true},
		{"host:", 0, 25565, "", 0, true},
		{"host:invalid", 0, 25565, "", 0, true},
		{"[::1]:8080", 0, 25565, "::1", 8080, false},
		{"[2001:db8::1]", 0, 25565, "2001:db8::1", 25565, false},
	}

	for _, test := range tests {
		host, port, err := parseAddress(test.addr, test.optPort, test.defaultPort)
		
		if test.expectError {
			if err == nil {
				t.Errorf("parseAddress(%s, %d, %d): expected error", test.addr, test.optPort, test.defaultPort)
			}
			continue
		}

		if err != nil {
			t.Errorf("parseAddress(%s, %d, %d): unexpected error: %v", test.addr, test.optPort, test.defaultPort, err)
			continue
		}

		if host != test.expectHost {
			t.Errorf("parseAddress(%s, %d, %d): expected host %s, got %s", test.addr, test.optPort, test.defaultPort, test.expectHost, host)
		}

		if port != test.expectPort {
			t.Errorf("parseAddress(%s, %d, %d): expected port %d, got %d", test.addr, test.optPort, test.defaultPort, test.expectPort, port)
		}
	}
}

func TestOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.Timeout != 5*time.Second {
		t.Errorf("Default timeout should be 5s, got %v", opts.Timeout)
	}
	if opts.Port != 0 {
		t.Errorf("Default port should be 0, got %d", opts.Port)
	}
	if opts.Players {
		t.Error("Default players should be false")
	}
	if opts.PortRange != nil {
		t.Error("Default port range should be nil")
	}
	if opts.MaxConcurrency != 0 {
		t.Error("Default max concurrency should be 0 (unlimited)")
	}

	// Test functional options
	opts = DefaultOptions()
	Timeout(10 * time.Second)(opts)
	Port(8080)(opts)
	WithPlayers()(opts)
	WithMaxConcurrency(5)(opts)

	if opts.Timeout != 10*time.Second {
		t.Errorf("Timeout option failed, got %v", opts.Timeout)
	}
	if opts.Port != 8080 {
		t.Errorf("Port option failed, got %d", opts.Port)
	}
	if !opts.Players {
		t.Error("WithPlayers option failed")
	}
	if opts.MaxConcurrency != 5 {
		t.Errorf("WithMaxConcurrency option failed, got %d", opts.MaxConcurrency)
	}
}

func TestPortRangeOptions(t *testing.T) {
	// Test WithPortRange
	opts := DefaultOptions()
	WithPortRange(25565, 25570)(opts)
	
	if len(opts.PortRange) != 6 {
		t.Errorf("Expected 6 ports in range, got %d", len(opts.PortRange))
	}
	
	expectedPorts := []int{25565, 25566, 25567, 25568, 25569, 25570}
	for i, port := range opts.PortRange {
		if port != expectedPorts[i] {
			t.Errorf("Port range index %d: expected %d, got %d", i, expectedPorts[i], port)
		}
	}
	
	// Test WithCustomPorts
	opts = DefaultOptions()
	customPorts := []int{25565, 27015, 7777}
	WithCustomPorts(customPorts)(opts)
	
	if len(opts.PortRange) != len(customPorts) {
		t.Errorf("Expected %d custom ports, got %d", len(customPorts), len(opts.PortRange))
	}
	
	for i, port := range opts.PortRange {
		if port != customPorts[i] {
			t.Errorf("Custom ports index %d: expected %d, got %d", i, customPorts[i], port)
		}
	}
}

func TestDiscoverServers(t *testing.T) {
	ctx := context.Background()
	
	// Test with offline server - should return empty list
	servers, err := DiscoverServers(ctx, "192.168.1.99", Timeout(1*time.Second), WithMaxConcurrency(5))
	if err != nil {
		t.Errorf("DiscoverServers should not return error for offline servers, got: %v", err)
	}
	
	if len(servers) != 0 {
		t.Errorf("Expected no servers found for offline address, got %d", len(servers))
	}
	
	// Test with specific port
	servers, err = DiscoverServers(ctx, "192.168.1.99:25565", Timeout(1*time.Second))
	if err != nil {
		t.Errorf("DiscoverServers with port should not return error, got: %v", err)
	}
	
	// Test with custom ports
	servers, err = DiscoverServers(ctx, "192.168.1.99", 
		Timeout(1*time.Second),
		WithCustomPorts([]int{25565, 27015}))
	if err != nil {
		t.Errorf("DiscoverServers with custom ports should not return error, got: %v", err)
	}
	
	// Test with port range
	servers, err = DiscoverServers(ctx, "192.168.1.99",
		Timeout(1*time.Second),
		WithPortRange(25565, 25567),
		WithMaxConcurrency(2))
	if err != nil {
		t.Errorf("DiscoverServers with port range should not return error, got: %v", err)
	}
}

func TestDiscoverServersWithRealServers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	ctx := context.Background()
	
	// Test discovering servers on localhost with all default ports
	servers, err := DiscoverServers(ctx, "localhost", Timeout(5*time.Second))
	if err != nil {
		t.Logf("DiscoverServers test failed: %v", err)
		return
	}
	
	t.Logf("Found %d server(s) on localhost", len(servers))
	for i, server := range servers {
		t.Logf("Server %d: %s on port %d (%s)", i+1, server.Game, server.Port, server.Name)
	}
	
	// Test with specific port range
	servers, err = DiscoverServers(ctx, "localhost",
		Timeout(5*time.Second),
		WithPortRange(25565, 25570))
	if err != nil {
		t.Logf("DiscoverServers with port range test failed: %v", err)
		return
	}
	
	t.Logf("Found %d server(s) in port range 25565-25570", len(servers))
}

// Integration test - only runs with real servers
func TestQueryWithRealMinecraftServer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	info, err := Query(ctx, "minecraft", "localhost:25565", Timeout(10*time.Second))
	if err != nil {
		t.Logf("Real Minecraft server test failed (server may not be running): %v", err)
		return
	}

	if !info.Online {
		t.Log("Minecraft server appears to be offline")
		return
	}

	// Validate response structure
	if info.Game != "minecraft" {
		t.Errorf("Expected game 'minecraft', got '%s'", info.Game)
	}

	if info.Name == "" {
		t.Log("Server name is empty (this is acceptable for some Minecraft servers)")
	}

	if info.Version == "" {
		t.Error("Server version should not be empty for online server")
	}

	t.Logf("Real Minecraft server test passed: %s (v%s)", info.Name, info.Version)
}

func TestAutoDetectWithRealServer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	info, err := AutoDetect(ctx, "localhost:25565", Timeout(10*time.Second))
	if err != nil {
		t.Logf("Auto-detect test failed (server may not be running): %v", err)
		return
	}

	if !info.Online {
		t.Log("Server appears to be offline")
		return
	}

	if info.Game == "" {
		t.Error("Auto-detect should determine game type")
	}

	t.Logf("Auto-detect test passed: detected %s server", info.Game)
}

func TestQueryWithPlayersRealServer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	info, err := Query(ctx, "minecraft", "localhost:25565", Timeout(10*time.Second), WithPlayers())
	if err != nil {
		t.Logf("Real server with players test failed: %v", err)
		return
	}

	if !info.Online {
		t.Log("Server appears to be offline")
		return
	}

	// Player list should be initialized even if empty
	if info.Players.List == nil {
		t.Error("Player list should be initialized when requesting players")
	}

	t.Logf("Player list test passed: %d players online", len(info.Players.List))
	for _, player := range info.Players.List {
		t.Logf("  - %s", player.Name)
	}
}

// Helper function
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Benchmark tests
func BenchmarkQuery(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Query(ctx, "minecraft", "192.168.1.99:25565", Timeout(1*time.Second))
	}
}

func BenchmarkAutoDetect(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = AutoDetect(ctx, "192.168.1.99:25565", Timeout(1*time.Second))
	}
}
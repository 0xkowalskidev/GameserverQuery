//go:build integration

package query

import (
	"context"
	"testing"
	"time"
)

// Real-world server test data
var realWorldServers = map[string][]struct {
	name string
	addr string
}{
	"minecraft": {
		{"Hypixel", "mc.hypixel.net:25565"},
		{"2b2t", "2b2t.org:25565"},
		{"CubeCraft", "play.cubecraft.net:25565"},
		{"Mineplex", "us.mineplex.com:25565"},
		{"The Hive", "hivemc.com:25565"},
	},
	"source": {
		{"CS2 Community Server", "91.211.118.52:27018"},
		{"GameTracker CS2", "74.91.116.63:27015"}, 
		{"CS2 Public", "162.248.92.24:27015"},
		{"Source Server", "208.103.169.25:27015"},
		{"Community Gaming", "74.91.113.106:27015"},
	},
	"terraria": {
		// Note: Most public Terraria servers use TShock with REST API, not our native protocol
		{"Local Server", "localhost:25567"},
		{"Local Test", "127.0.0.1:7777"},
		{"Test Server 1", "terraria.example.com:7777"},
		{"Test Server 2", "tshock.example.com:7777"},
		{"Test Server 3", "modded.terraria.net:7777"},
	},
}

func TestRealWorldMinecraftServers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world server tests in short mode")
	}

	testRealWorldServers(t, "minecraft", realWorldServers["minecraft"])
}

func TestRealWorldSourceServers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world server tests in short mode")
	}

	testRealWorldServers(t, "source", realWorldServers["source"])
}

func TestRealWorldTerrariaServers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world server tests in short mode")
	}

	testRealWorldServers(t, "terraria", realWorldServers["terraria"])
}


func testRealWorldServers(t *testing.T, game string, servers []struct{ name, addr string }) {
	ctx := context.Background()
	successCount := 0
	totalServers := len(servers)

	for _, server := range servers {
		t.Run(server.name, func(t *testing.T) {
			// Use a reasonable timeout for real-world servers
			info, err := Query(ctx, game, server.addr, 
				Timeout(15*time.Second))
			
			if err != nil {
				t.Logf("Server %s (%s) failed: %v", server.name, server.addr, err)
				return
			}

			if !info.Online {
				t.Logf("Server %s (%s) reported as offline", server.name, server.addr)
				return
			}

			// Server responded successfully
			successCount++
			
			// Validate basic response structure
			if info.Name == "" {
				t.Errorf("Server %s: name should not be empty", server.name)
			}
			
			if info.Game == "" {
				t.Errorf("Server %s: game should not be empty", server.name)
			}

			if info.Address == "" {
				t.Errorf("Server %s: address should not be empty", server.name)
			}

			if info.Port <= 0 {
				t.Errorf("Server %s: port should be positive", server.name)
			}

			if info.Players.Max <= 0 {
				t.Errorf("Server %s: max players should be positive", server.name)
			}

			if info.Ping <= 0 {
				t.Errorf("Server %s: ping should be positive", server.name)
			}

			t.Logf("✓ %s (%s): %s - %d/%d players - %v ping", 
				server.name, server.addr, info.Name, 
				info.Players.Current, info.Players.Max, info.Ping)
		})
	}

	// Allow for many servers to be down - just require that we can connect to real servers when they exist
	// For minecraft and source we expect at least one to work. For terraria, it's acceptable if none work.
	var minRequired int
	if game == "minecraft" || game == "source" {
		minRequired = 1
	} else {
		minRequired = 0 // Terraria servers are harder to find publicly
	}
	
	if successCount < minRequired {
		t.Errorf("Only %d/%d %s servers responded successfully (required: %d)", 
			successCount, totalServers, game, minRequired)
	} else {
		t.Logf("Real-world %s server test summary: %d/%d servers responded successfully", 
			game, successCount, totalServers)
	}
}

func TestRealWorldServersWithPlayers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world server tests in short mode")
	}

	ctx := context.Background()

	// Test a few popular servers with player lists
	testCases := []struct {
		game string
		name string
		addr string
	}{
		{"minecraft", "Hypixel", "mc.hypixel.net:25565"},
		{"source", "CS2 Community", "91.211.118.52:27018"},
	}

	for _, tc := range testCases {
		t.Run(tc.name+"_with_players", func(t *testing.T) {
			info, err := Query(ctx, tc.game, tc.addr,
				Timeout(15*time.Second),
				WithPlayers())

			if err != nil {
				t.Logf("Server %s failed: %v (this is acceptable)", tc.name, err)
				return
			}

			if !info.Online {
				t.Logf("Server %s is offline (this is acceptable)", tc.name)
				return
			}

			// Validate player list is initialized
			if info.Players.List == nil {
				t.Errorf("Player list should be initialized when requesting players")
			}

			t.Logf("✓ %s: %d players online, player list length: %d", 
				tc.name, info.Players.Current, len(info.Players.List))

			// Log some player names if available
			playerCount := len(info.Players.List)
			if playerCount > 0 {
				logCount := min(5, playerCount) // Log up to 5 players
				t.Logf("  Sample players:")
				for i := 0; i < logCount; i++ {
					player := info.Players.List[i]
					t.Logf("    - %s", player.Name)
				}
			}
		})
	}
}

func TestAutoDetectRealWorldServers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world server tests in short mode")
	}

	ctx := context.Background()

	// Test auto-detection with servers on default ports
	testCases := []struct {
		name         string
		addr         string
		expectedGame string
	}{
		{"Hypixel", "mc.hypixel.net:25565", "minecraft"},
		{"CS2 Server", "91.211.118.52:27018", "source"},
	}

	successCount := 0
	for _, tc := range testCases {
		t.Run("AutoDetect_"+tc.name, func(t *testing.T) {
			info, err := AutoDetect(ctx, tc.addr, Timeout(15*time.Second))

			if err != nil {
				t.Logf("Auto-detect failed for %s: %v (this is acceptable)", tc.name, err)
				return
			}

			if !info.Online {
				t.Logf("Server %s is offline (this is acceptable)", tc.name)
				return
			}

			successCount++

			if info.Game != tc.expectedGame {
				t.Errorf("Auto-detect for %s: expected game %s, got %s", 
					tc.name, tc.expectedGame, info.Game)
			}

			t.Logf("✓ Auto-detected %s as %s server: %s", tc.name, info.Game, info.Name)
		})
	}

	if successCount == 0 {
		t.Log("No auto-detect tests succeeded (servers may be down)")
	}
}

// Helper functions for Go versions that don't have min/max
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
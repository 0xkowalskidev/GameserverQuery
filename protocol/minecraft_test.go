package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Helper functions for creating test data
func createMinecraftStatus(version, versionName string, protocol, playersOnline, playersMax int, description interface{}) MinecraftStatus {
	return MinecraftStatus{
		Version: struct {
			Name     string `json:"name"`
			Protocol int    `json:"protocol"`
		}{
			Name:     versionName,
			Protocol: protocol,
		},
		Players: struct {
			Max    int `json:"max"`
			Online int `json:"online"`
			Sample []struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			} `json:"sample,omitempty"`
		}{
			Max:    playersMax,
			Online: playersOnline,
		},
		Description: description,
	}
}

func withPlayerSample(status *MinecraftStatus, players ...string) {
	for i := 0; i < len(players); i += 2 {
		if i+1 < len(players) {
			status.Players.Sample = append(status.Players.Sample, struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			}{
				Name: players[i],
				ID:   players[i+1],
			})
		}
	}
}

func withFavicon(status *MinecraftStatus, favicon string) {
	status.Favicon = favicon
}

// mockMinecraftServer simulates a Minecraft server for testing purposes.
type mockMinecraftServer struct {
	t        *testing.T
	listener net.Listener
	response MinecraftStatus
}

// newMockMinecraftServer creates and starts a new mock server.
func newMockMinecraftServer(t *testing.T, response MinecraftStatus) *mockMinecraftServer {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start mock server: %v", err)
	}

	server := &mockMinecraftServer{
		t:        t,
		listener: l,
		response: response,
	}

	go server.handleConnections()
	return server
}

// Addr returns the address of the mock server.
func (s *mockMinecraftServer) Addr() string {
	return s.listener.Addr().String()
}

// Close stops the mock server.
func (s *mockMinecraftServer) Close() {
	s.listener.Close()
}

// handleConnections accepts and handles incoming connections.
func (s *mockMinecraftServer) handleConnections() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // Listener closed
		}
		go s.handleRequest(conn)
	}
}

// handleRequest processes a single client request.
func (s *mockMinecraftServer) handleRequest(conn net.Conn) {
	defer conn.Close()

	// 1. Read Handshake
	p := &MinecraftProtocol{}
	_, err := p.readVarIntPrefixedData(conn)
	if err != nil {
		s.t.Logf("Error reading handshake: %v", err)
		return
	}

	// 2. Read Status Request
	_, err = p.readVarIntPrefixedData(conn)
	if err != nil {
		s.t.Logf("Error reading status request: %v", err)
		return
	}

	// 3. Write Status Response
	jsonResponse, err := json.Marshal(s.response)
	if err != nil {
		s.t.Fatalf("Failed to marshal response: %v", err)
	}

	// Construct the response payload: Packet ID (0x00) + JSON Data
	var payload bytes.Buffer
	p.writeVarInt(&payload, 0x00) // Packet ID
	p.writeString(&payload, string(jsonResponse))

	// Send the payload with a length prefix
	if err := p.writeVarIntPrefixedData(conn, payload.Bytes()); err != nil {
		s.t.Logf("Error writing response: %v", err)
	}
}

func TestMinecraftProtocol_Query(t *testing.T) {
	// 1. Setup mock server with a realistic response
	mockResponse := createMinecraftStatus("", "1.19.4", 762, 5, 100, "A Minecraft Server")
	withPlayerSample(&mockResponse, "Player1", "uuid1", "Player2", "uuid2")

	server := newMockMinecraftServer(t, mockResponse)
	defer server.Close()

	// 2. Query the mock server
	protocol := &MinecraftProtocol{}
	opts := &Options{
		Timeout: 5 * time.Second,
		Players: true,
	}
	info, err := protocol.Query(context.Background(), server.Addr(), opts)

	// 3. Assert all returned fields
	assert.NoError(t, err)
	assertServerInfo(t, info, expectedServerInfo{
		online:         true,
		game:           "minecraft",
		name:           "A Minecraft Server",
		version:        "1.19.4",
		playersCurrent: 5,
		playersMax:     100,
		playerNames:    []string{"Player1", "Player2"},
	})
}

func TestMinecraftProtocol_Query_ComplexMOTD(t *testing.T) {
	// 1. Setup mock server with a complex MOTD
	complexMOTD := map[string]interface{}{
		"extra": []interface{}{
			map[string]interface{}{"text": "A "},
			map[string]interface{}{"text": "Multi-Line", "color": "gold"},
			map[string]interface{}{"text": "\n"},
			map[string]interface{}{"text": "§cMOTD!", "bold": true},
		},
		"text": "Welcome!",
	}
	mockResponse := createMinecraftStatus("", "1.20.1", 763, 1, 20, complexMOTD)

	server := newMockMinecraftServer(t, mockResponse)
	defer server.Close()

	// 2. Query the mock server
	protocol := &MinecraftProtocol{}
	opts := &Options{
		Timeout: 5 * time.Second,
	}
	info, err := protocol.Query(context.Background(), server.Addr(), opts)

	// 3. Assert the results
	assert.NoError(t, err)
	assertServerInfo(t, info, expectedServerInfo{
		online:         true,
		game:           "minecraft",
		name:           "Welcome!A Multi-Line\nMOTD!",
		version:        "1.20.1",
		playersCurrent: 1,
		playersMax:     20,
	})
}

func TestMinecraftProtocol_Query_EmptyPlayerList(t *testing.T) {
	// 1. Setup mock server with no player sample
	mockResponse := createMinecraftStatus("", "1.20.1", 763, 0, 50, "Empty Server")
	// No withPlayerSample call - empty by default

	server := newMockMinecraftServer(t, mockResponse)
	defer server.Close()

	// 2. Query the mock server with players enabled
	protocol := &MinecraftProtocol{}
	opts := &Options{
		Timeout: 5 * time.Second,
		Players: true,
	}
	info, err := protocol.Query(context.Background(), server.Addr(), opts)

	// 3. Assert the results
	assert.NoError(t, err)
	assertServerInfo(t, info, expectedServerInfo{
		online:         true,
		game:           "minecraft",
		name:           "Empty Server",
		version:        "1.20.1",
		playersCurrent: 0,
		playersMax:     50,
		playerNames:    []string{}, // Empty list
	})
}

func TestMinecraftProtocol_Query_NoPlayersRequested(t *testing.T) {
	// 1. Setup mock server with player sample
	mockResponse := createMinecraftStatus("", "1.19.4", 762, 25, 100, "Test Server")
	withPlayerSample(&mockResponse, "TestPlayer", "test-uuid")

	server := newMockMinecraftServer(t, mockResponse)
	defer server.Close()

	// 2. Query the mock server WITHOUT requesting players
	protocol := &MinecraftProtocol{}
	opts := &Options{
		Timeout: 5 * time.Second,
		Players: false, // Don't request player list
	}
	info, err := protocol.Query(context.Background(), server.Addr(), opts)

	// 3. Assert the results
	assert.NoError(t, err)
	assertServerInfo(t, info, expectedServerInfo{
		online:         true,
		game:           "minecraft",
		name:           "Test Server",
		version:        "1.19.4",
		playersCurrent: 25,
		playersMax:     100,
		playerNames:    nil, // nil when not requested
	})
}

// Helper struct for expected server info values
type expectedServerInfo struct {
	online         bool
	game           string
	name           string
	version        string
	playersCurrent int
	playersMax     int
	playerNames    []string // nil means don't check, empty slice means check for empty
}

// assertServerInfo validates all ServerInfo fields with sensible defaults
func assertServerInfo(t *testing.T, info *ServerInfo, expected expectedServerInfo) {
	assert.NotNil(t, info, "ServerInfo should not be nil")
	
	// Basic fields
	assert.Equal(t, expected.online, info.Online)
	assert.Equal(t, expected.game, info.Game)
	assert.Equal(t, expected.name, info.Name)
	assert.Equal(t, expected.version, info.Version)
	
	// Fields not set by Minecraft protocol
	assert.Empty(t, info.Address, "Address not set by protocol")
	assert.Zero(t, info.Port, "Port not set by protocol")
	assert.Empty(t, info.Map, "Map field not used by Minecraft")
	assert.Zero(t, info.Ping, "Ping not measured in test implementation")
	assert.Nil(t, info.Extra, "Extra fields should be nil")
	
	// Player information
	assert.Equal(t, expected.playersCurrent, info.Players.Current)
	assert.Equal(t, expected.playersMax, info.Players.Max)
	
	// Player list validation
	if expected.playerNames != nil {
		assert.Len(t, info.Players.List, len(expected.playerNames))
		for i, name := range expected.playerNames {
			assert.Equal(t, name, info.Players.List[i].Name)
			// Default player fields
			assert.Zero(t, info.Players.List[i].Score)
			assert.Zero(t, info.Players.List[i].Duration)
		}
	} else {
		assert.Nil(t, info.Players.List)
	}
}

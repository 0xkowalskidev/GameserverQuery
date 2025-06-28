package protocol

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Helper functions for creating test data
func createA2SInfo(name, mapName, folder, game, version string, appID uint16, players, maxPlayers uint8) A2SInfo {
	return A2SInfo{
		Protocol:    0x11,
		Name:        name,
		Map:         mapName,
		Folder:      folder,
		Game:        game,
		AppID:       appID,
		Players:     players,
		MaxPlayers:  maxPlayers,
		Bots:        0,
		ServerType:  'd',
		Environment: 'l',
		Visibility:  0,
		VAC:         1,
		Version:     version,
	}
}

// mockSourceServer simulates a Source server for testing purposes.
type mockSourceServer struct {
	t                *testing.T
	listener         net.PacketConn
	infoResponse     A2SInfo
	players          []sourcePlayer
	requireChallenge bool
	challengeValue   uint32
}

type sourcePlayer struct {
	name     string
	score    int32
	duration float32
}

// newMockSourceServer creates and starts a new mock server.
func newMockSourceServer(t *testing.T, infoResponse A2SInfo) *mockSourceServer {
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start mock server: %v", err)
	}

	server := &mockSourceServer{
		t:              t,
		listener:       l,
		infoResponse:   infoResponse,
		challengeValue: 0x12345678,
	}

	go server.handleRequests()
	return server
}

// Addr returns the address of the mock server.
func (s *mockSourceServer) Addr() string {
	return s.listener.LocalAddr().String()
}

// Close stops the mock server.
func (s *mockSourceServer) Close() {
	s.listener.Close()
}

// setPlayers sets the player list for the mock server.
func (s *mockSourceServer) setPlayers(players []sourcePlayer) {
	s.players = players
}

// setRequireChallenge configures whether the server requires challenge for A2S_INFO.
func (s *mockSourceServer) setRequireChallenge(require bool) {
	s.requireChallenge = require
}

// handleRequests processes incoming UDP packets.
func (s *mockSourceServer) handleRequests() {
	buffer := make([]byte, 1400)
	for {
		n, addr, err := s.listener.ReadFrom(buffer)
		if err != nil {
			return // Listener closed
		}
		go s.handlePacket(buffer[:n], addr)
	}
}

// handlePacket processes a single packet.
func (s *mockSourceServer) handlePacket(data []byte, addr net.Addr) {
	if len(data) < 5 {
		return
	}

	// Check for Source header
	if data[0] != 0xFF || data[1] != 0xFF || data[2] != 0xFF || data[3] != 0xFF {
		return
	}

	// Add a small delay to simulate network latency
	time.Sleep(5 * time.Millisecond)

	switch data[4] {
	case 0x54: // A2S_INFO
		s.handleInfoRequest(data, addr)
	case 0x55: // A2S_PLAYER
		s.handlePlayerRequest(data, addr)
	}
}

// handleInfoRequest handles A2S_INFO requests.
func (s *mockSourceServer) handleInfoRequest(data []byte, addr net.Addr) {
	// Check if challenge is present and required
	if s.requireChallenge && len(data) < 24 {
		// Send challenge response
		var response bytes.Buffer
		response.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x41}) // Challenge header
		binary.Write(&response, binary.LittleEndian, s.challengeValue)
		s.listener.WriteTo(response.Bytes(), addr)
		return
	}

	// Build A2S_INFO response
	var response bytes.Buffer
	response.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x49}) // A2S_INFO response header
	
	// Protocol version
	response.WriteByte(s.infoResponse.Protocol)
	
	// Name
	response.WriteString(s.infoResponse.Name)
	response.WriteByte(0)
	
	// Map
	response.WriteString(s.infoResponse.Map)
	response.WriteByte(0)
	
	// Folder
	response.WriteString(s.infoResponse.Folder)
	response.WriteByte(0)
	
	// Game
	response.WriteString(s.infoResponse.Game)
	response.WriteByte(0)
	
	// App ID
	binary.Write(&response, binary.LittleEndian, s.infoResponse.AppID)
	
	// Players
	response.WriteByte(s.infoResponse.Players)
	
	// Max Players
	response.WriteByte(s.infoResponse.MaxPlayers)
	
	// Bots
	response.WriteByte(s.infoResponse.Bots)
	
	// Server type
	response.WriteByte(s.infoResponse.ServerType)
	
	// Environment
	response.WriteByte(s.infoResponse.Environment)
	
	// Visibility
	response.WriteByte(s.infoResponse.Visibility)
	
	// VAC
	response.WriteByte(s.infoResponse.VAC)
	
	// Version
	response.WriteString(s.infoResponse.Version)
	response.WriteByte(0)

	s.listener.WriteTo(response.Bytes(), addr)
}

// handlePlayerRequest handles A2S_PLAYER requests.
func (s *mockSourceServer) handlePlayerRequest(data []byte, addr net.Addr) {
	if len(data) < 9 {
		return
	}

	// Check challenge
	challenge := binary.LittleEndian.Uint32(data[5:9])
	if challenge == 0xFFFFFFFF {
		// Send challenge response
		var response bytes.Buffer
		response.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x41}) // Challenge header
		binary.Write(&response, binary.LittleEndian, s.challengeValue)
		s.listener.WriteTo(response.Bytes(), addr)
		return
	}

	// Build A2S_PLAYER response
	var response bytes.Buffer
	response.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x44}) // A2S_PLAYER response header
	
	// Player count
	response.WriteByte(byte(len(s.players)))
	
	// Players
	for i, player := range s.players {
		response.WriteByte(byte(i)) // Index
		response.WriteString(player.name)
		response.WriteByte(0)
		binary.Write(&response, binary.LittleEndian, player.score)
		// Duration as float32 in little endian
		bits := math.Float32bits(player.duration)
		binary.Write(&response, binary.LittleEndian, bits)
	}

	s.listener.WriteTo(response.Bytes(), addr)
}

func TestSourceProtocol_Query(t *testing.T) {
	// 1. Setup mock server with a CS:GO response
	mockResponse := createA2SInfo(
		"Test CS:GO Server",
		"de_dust2",
		"csgo",
		"Counter-Strike: Global Offensive",
		"1.0",
		730,
		16,
		32,
	)

	server := newMockSourceServer(t, mockResponse)
	defer server.Close()

	// 2. Query the mock server
	protocol := &SourceProtocol{}
	opts := &Options{
		Timeout: 5 * time.Second,
		Players: false,
	}
	info, err := protocol.Query(context.Background(), server.Addr(), opts)

	// 3. Assert all returned fields
	assert.NoError(t, err)
	assertSourceServerInfo(t, info, expectedSourceServerInfo{
		online:         true,
		name:           "Test CS:GO Server",
		game:           "counter-strike",
		map_:           "de_dust2",
		version:        "1.0",
		playersCurrent: 16,
		playersMax:     32,
	})
}

func TestSourceProtocol_Query_WithChallenge(t *testing.T) {
	// 1. Setup mock server that requires challenge
	mockResponse := createA2SInfo(
		"Challenged Server",
		"gm_construct",
		"garrysmod",
		"Garry's Mod",
		"2023.06.28",
		4000,
		10,
		50,
	)

	server := newMockSourceServer(t, mockResponse)
	server.setRequireChallenge(true)
	defer server.Close()

	// 2. Query the mock server
	protocol := &SourceProtocol{}
	opts := &Options{
		Timeout: 5 * time.Second,
	}
	info, err := protocol.Query(context.Background(), server.Addr(), opts)

	// 3. Assert the results
	assert.NoError(t, err)
	assertSourceServerInfo(t, info, expectedSourceServerInfo{
		online:         true,
		name:           "Challenged Server",
		game:           "garrys-mod",
		map_:           "gm_construct",
		version:        "2023.06.28",
		playersCurrent: 10,
		playersMax:     50,
	})
}

func TestSourceProtocol_Query_WithPlayers(t *testing.T) {
	// 1. Setup mock server with players
	mockResponse := createA2SInfo(
		"TF2 Server",
		"cp_dustbowl",
		"tf",
		"Team Fortress 2",
		"1.5.2.1",
		440,
		24,
		32,
	)

	server := newMockSourceServer(t, mockResponse)
	server.setPlayers([]sourcePlayer{
		{name: "Player1", score: 100, duration: 3600},
		{name: "Player2", score: 50, duration: 1800},
		{name: "Player3", score: 75, duration: 900},
	})
	defer server.Close()

	// 2. Query the mock server with players enabled
	protocol := &SourceProtocol{}
	opts := &Options{
		Timeout: 5 * time.Second,
		Players: true,
	}
	info, err := protocol.Query(context.Background(), server.Addr(), opts)

	// 3. Assert the results
	assert.NoError(t, err)
	assertSourceServerInfo(t, info, expectedSourceServerInfo{
		online:         true,
		name:           "TF2 Server",
		game:           "team-fortress-2",
		map_:           "cp_dustbowl",
		version:        "1.5.2.1",
		playersCurrent: 24,
		playersMax:     32,
		playerNames:    []string{"Player1", "Player2", "Player3"},
		playerScores:   []int{100, 50, 75},
		playerDurations: []time.Duration{
			time.Duration(3600 * time.Second),
			time.Duration(1800 * time.Second),
			time.Duration(900 * time.Second),
		},
	})
}

func TestSourceProtocol_Query_EmptyPlayerList(t *testing.T) {
	// 1. Setup mock server with no players
	mockResponse := createA2SInfo(
		"Empty Server",
		"dm_lockdown",
		"hl2mp",
		"Half-Life 2: Deathmatch",
		"1.0.0",
		320,
		0,
		16,
	)

	server := newMockSourceServer(t, mockResponse)
	server.setPlayers([]sourcePlayer{})
	defer server.Close()

	// 2. Query the mock server with players enabled
	protocol := &SourceProtocol{}
	opts := &Options{
		Timeout: 5 * time.Second,
		Players: true,
	}
	info, err := protocol.Query(context.Background(), server.Addr(), opts)

	// 3. Assert the results
	assert.NoError(t, err)
	assertSourceServerInfo(t, info, expectedSourceServerInfo{
		online:         true,
		name:           "Empty Server",
		game:           "half-life",
		map_:           "dm_lockdown",
		version:        "1.0.0",
		playersCurrent: 0,
		playersMax:     16,
		playerNames:    []string{}, // Empty list
	})
}

func TestSourceProtocol_GameDetection(t *testing.T) {
	tests := []struct {
		name        string
		gameDesc    string
		appID       uint16
		expectedGame string
	}{
		{
			name:        "Counter-Strike by AppID",
			gameDesc:    "Counter-Strike",
			appID:       730,
			expectedGame: "counter-strike",
		},
		{
			name:        "Counter-Strike 2 by description",
			gameDesc:    "Counter-Strike 2",
			appID:       0,
			expectedGame: "counter-strike-2",
		},
		{
			name:        "Rust by description",
			gameDesc:    "Rust",
			appID:       0, // Rust's actual AppID exceeds uint16
			expectedGame: "rust",
		},
		{
			name:        "Garry's Mod variant spelling",
			gameDesc:    "GarrysMod",
			appID:       0,
			expectedGame: "garrys-mod",
		},
		{
			name:        "Unknown game",
			gameDesc:    "Some Unknown Game",
			appID:       0,
			expectedGame: "source",
		},
	}

	protocol := &SourceProtocol{}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := protocol.detectGameType(tt.gameDesc, tt.appID)
			assert.Equal(t, tt.expectedGame, result)
		})
	}
}

// Helper struct for expected server info values
type expectedSourceServerInfo struct {
	online          bool
	name            string
	game            string
	map_            string
	version         string
	playersCurrent  int
	playersMax      int
	playerNames     []string
	playerScores    []int
	playerDurations []time.Duration
}

// assertSourceServerInfo validates all ServerInfo fields
func assertSourceServerInfo(t *testing.T, info *ServerInfo, expected expectedSourceServerInfo) {
	assert.NotNil(t, info, "ServerInfo should not be nil")
	
	// Basic fields
	assert.Equal(t, expected.online, info.Online)
	assert.Equal(t, expected.name, info.Name)
	assert.Equal(t, expected.game, info.Game)
	assert.Equal(t, expected.map_, info.Map)
	assert.Equal(t, expected.version, info.Version)
	
	// Fields not set by Source protocol
	assert.Empty(t, info.MOTD, "Source protocol doesn't set MOTD")
	assert.Empty(t, info.Address, "Address not set by protocol")
	assert.Zero(t, info.Port, "Port not set by protocol")
	assert.GreaterOrEqual(t, info.Ping, 0, "Ping should be non-negative")
	assert.Nil(t, info.Extra, "Extra fields should be nil")
	
	// Player information
	assert.Equal(t, expected.playersCurrent, info.Players.Current)
	assert.Equal(t, expected.playersMax, info.Players.Max)
	
	// Player list validation
	if expected.playerNames != nil {
		assert.Len(t, info.Players.List, len(expected.playerNames))
		for i, name := range expected.playerNames {
			assert.Equal(t, name, info.Players.List[i].Name)
			if expected.playerScores != nil && i < len(expected.playerScores) {
				assert.Equal(t, expected.playerScores[i], info.Players.List[i].Score)
			}
			if expected.playerDurations != nil && i < len(expected.playerDurations) {
				assert.Equal(t, expected.playerDurations[i], info.Players.List[i].Duration)
			}
		}
	} else {
		assert.Nil(t, info.Players.List)
	}
}
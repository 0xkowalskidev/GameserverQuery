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
	mockResponse := MinecraftStatus{
		Version: struct {
			Name     string `json:"name"`
			Protocol int    `json:"protocol"`
		}{
			Name:     "1.19.4",
			Protocol: 762,
		},
		Players: struct {
			Max    int `json:"max"`
			Online int `json:"online"`
			Sample []struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			} `json:"sample,omitempty"`
		}{
			Max:    100,
			Online: 5,
			Sample: []struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			}{
				{Name: "Player1", ID: "uuid1"},
				{Name: "Player2", ID: "uuid2"},
			},
		},
		Description: "A Minecraft Server",
	}

	server := newMockMinecraftServer(t, mockResponse)
	defer server.Close()

	// 2. Query the mock server
	protocol := &MinecraftProtocol{}
	opts := &Options{
		Timeout: 5 * time.Second,
		Players: true,
	}
	info, err := protocol.Query(context.Background(), server.Addr(), opts)

	// 3. Assert the results
	assert.NoError(t, err)
	assert.True(t, info.Online)
	assert.Equal(t, "A Minecraft Server", info.MOTD)
	assert.Equal(t, "1.19.4", info.Version)
	assert.Equal(t, 5, info.Players.Current)
	assert.Equal(t, 100, info.Players.Max)
	assert.Len(t, info.Players.List, 2)
	assert.Equal(t, "Player1", info.Players.List[0].Name)
	assert.Equal(t, "Player2", info.Players.List[1].Name)
}

func TestMinecraftProtocol_Query_ComplexMOTD(t *testing.T) {
	// 1. Setup mock server with a complex MOTD
	mockResponse := MinecraftStatus{
		Version: struct {
			Name     string `json:"name"`
			Protocol int    `json:"protocol"`
		}{
			Name:     "1.20.1",
			Protocol: 763,
		},
		Players: struct {
			Max    int `json:"max"`
			Online int `json:"online"`
			Sample []struct {
				Name string `json:"name"`	
				ID   string `json:"id"`
			} `json:"sample,omitempty"`
		}{
			Max:    20,
			Online: 1,
		},
		Description: map[string]interface{}{
			"extra": []interface{}{
				map[string]interface{}{"text": "A "},
				map[string]interface{}{"text": "Multi-Line", "color": "gold"},
				map[string]interface{}{"text": "\n"},
				map[string]interface{}{"text": "§cMOTD!", "bold": true},
			},
			"text": "Welcome!",
		},
	}

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
	assert.True(t, info.Online)
	assert.Equal(t, "Welcome!A Multi-Line\nMOTD!", info.MOTD)
	assert.Equal(t, "1.20.1", info.Version)
}

package protocol

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

// SourceProtocol implements the Source A2S_INFO protocol
type SourceProtocol struct{}

func init() {
	registry.Register(&SourceProtocol{})
	
	// Register game aliases (slugified game names)
	registry.RegisterAlias("counter-strike-2", "source")
	registry.RegisterAlias("counter-strike", "source") // CS:GO is "Counter-Strike"
	registry.RegisterAlias("counter-source", "source")
	registry.RegisterAlias("garrys-mod", "source")
	registry.RegisterAlias("team-fortress-2", "source")
	registry.RegisterAlias("left-4-dead", "source")
	registry.RegisterAlias("left-4-dead-2", "source")
	registry.RegisterAlias("half-life", "source")
	registry.RegisterAlias("rust", "source")
	registry.RegisterAlias("ark-survival-evolved", "source")
	registry.RegisterAlias("insurgency", "source")
	registry.RegisterAlias("day-of-defeat", "source")
	registry.RegisterAlias("project-zomboid", "source")
	registry.RegisterAlias("valheim", "source")
	registry.RegisterAlias("satisfactory", "source")
	registry.RegisterAlias("7-days-to-die", "source")
}

func (s *SourceProtocol) Name() string {
	return "source"
}

func (s *SourceProtocol) DefaultPort() int {
	return 27015
}

func (s *SourceProtocol) Query(ctx context.Context, addr string, opts *Options) (*ServerInfo, error) {
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	// Set deadline
	conn.SetDeadline(time.Now().Add(opts.Timeout))

	start := time.Now()

	// Build A2S_INFO request
	request := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x54}
	request = append(request, []byte("Source Engine Query\x00")...)

	// Send request
	if _, err := conn.Write(request); err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("write failed: %w", err)
	}

	// Read response
	response := make([]byte, 1400)
	n, err := conn.Read(response)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("read failed: %w", err)
	}

	ping := time.Since(start)

	if n < 5 {
		return &ServerInfo{Online: false}, fmt.Errorf("response too short")
	}

	// Check for challenge response
	if response[4] == 0x41 { // Challenge response
		if n < 9 {
			return &ServerInfo{Online: false}, fmt.Errorf("challenge response too short")
		}
		challenge := binary.LittleEndian.Uint32(response[5:9])
		return s.queryWithChallenge(conn, addr, challenge, opts.Timeout, start, opts)
	}

	// Check for A2S_INFO response
	if response[4] != 0x49 {
		return &ServerInfo{Online: false}, fmt.Errorf("unexpected response type: %02x", response[4])
	}

	// Parse A2S_INFO response
	info, err := s.parseA2SInfoResponse(response[5:n])
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("parse failed: %w", err)
	}

	result := &ServerInfo{
		Name:    info.Name,
		Map:     info.Map,
		Version: info.Version,
		Online:  true,
		Players: PlayerInfo{
			Current: int(info.Players),
			Max:     int(info.MaxPlayers),
		},
		Ping: ping,
	}

	// Determine specific game based on game description and App ID
	result.Game = s.detectGameType(info.Game, info.AppID)

	// Query players if requested
	if opts.Players {
		players, err := s.queryPlayers(conn, addr, opts.Timeout)
		if err == nil {
			result.Players.List = players
		} else {
			result.Players.List = make([]Player, 0)
		}
	}

	return result, nil
}

func (s *SourceProtocol) queryWithChallenge(conn net.Conn, addr string, challenge uint32, timeout time.Duration, start time.Time, opts *Options) (*ServerInfo, error) {
	// Build A2S_INFO request with challenge
	request := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x54}
	request = append(request, []byte("Source Engine Query\x00")...)
	challengeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(challengeBytes, challenge)
	request = append(request, challengeBytes...)

	// Send request with challenge
	if _, err := conn.Write(request); err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("write challenge failed: %w", err)
	}

	// Read response
	response := make([]byte, 1400)
	n, err := conn.Read(response)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("read challenge response failed: %w", err)
	}

	ping := time.Since(start)

	if n < 5 || response[4] != 0x49 {
		return &ServerInfo{Online: false}, fmt.Errorf("invalid challenge response")
	}

	// Parse A2S_INFO response
	info, err := s.parseA2SInfoResponse(response[5:n])
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("parse challenge response failed: %w", err)
	}

	result := &ServerInfo{
		Name:    info.Name,
		Map:     info.Map,
		Version: info.Version,
		Online:  true,
		Players: PlayerInfo{
			Current: int(info.Players),
			Max:     int(info.MaxPlayers),
		},
		Ping: ping,
	}

	// Determine specific game
	result.Game = s.detectGameType(info.Game, info.AppID)

	// Query players if requested
	if opts.Players {
		players, err := s.queryPlayers(conn, addr, timeout)
		if err == nil {
			result.Players.List = players
		} else {
			result.Players.List = make([]Player, 0)
		}
	}

	return result, nil
}

func (s *SourceProtocol) queryPlayers(conn net.Conn, addr string, timeout time.Duration) ([]Player, error) {
	// A2S_PLAYER request
	request := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x55}
	challengeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(challengeBytes, 0xFFFFFFFF)
	request = append(request, challengeBytes...)

	conn.SetDeadline(time.Now().Add(timeout))

	// Send request
	if _, err := conn.Write(request); err != nil {
		return nil, err
	}

	// Read response
	response := make([]byte, 1400)
	n, err := conn.Read(response)
	if err != nil {
		return nil, err
	}

	if n < 5 {
		return nil, fmt.Errorf("player response too short")
	}

	// Check for challenge
	if response[4] == 0x41 {
		if n < 9 {
			return nil, fmt.Errorf("player challenge too short")
		}
		challenge := binary.LittleEndian.Uint32(response[5:9])
		
		// Retry with challenge
		request = []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x55}
		challengeBytes = make([]byte, 4)
		binary.LittleEndian.PutUint32(challengeBytes, challenge)
		request = append(request, challengeBytes...)

		if _, err := conn.Write(request); err != nil {
			return nil, err
		}

		n, err = conn.Read(response)
		if err != nil {
			return nil, err
		}
	}

	if n < 6 || response[4] != 0x44 {
		return nil, fmt.Errorf("invalid player response")
	}

	return s.parsePlayersResponse(response[5:n])
}

func (s *SourceProtocol) parseA2SInfoResponse(data []byte) (*A2SInfo, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("data too short")
	}

	info := &A2SInfo{}
	offset := 0

	// Protocol version
	if offset >= len(data) {
		return nil, fmt.Errorf("missing protocol version")
	}
	info.Protocol = data[offset]
	offset++

	// Name
	name, newOffset, err := s.readNullTerminatedString(data, offset)
	if err != nil {
		return nil, fmt.Errorf("read name failed: %w", err)
	}
	info.Name = name
	offset = newOffset

	// Map
	mapName, newOffset, err := s.readNullTerminatedString(data, offset)
	if err != nil {
		return nil, fmt.Errorf("read map failed: %w", err)
	}
	info.Map = mapName
	offset = newOffset

	// Folder
	folder, newOffset, err := s.readNullTerminatedString(data, offset)
	if err != nil {
		return nil, fmt.Errorf("read folder failed: %w", err)
	}
	info.Folder = folder
	offset = newOffset

	// Game
	game, newOffset, err := s.readNullTerminatedString(data, offset)
	if err != nil {
		return nil, fmt.Errorf("read game failed: %w", err)
	}
	info.Game = game
	offset = newOffset

	// App ID (2 bytes)
	if offset+1 >= len(data) {
		return nil, fmt.Errorf("missing app ID")
	}
	info.AppID = binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2

	// Players
	if offset >= len(data) {
		return nil, fmt.Errorf("missing players")
	}
	info.Players = data[offset]
	offset++

	// Max players
	if offset >= len(data) {
		return nil, fmt.Errorf("missing max players")
	}
	info.MaxPlayers = data[offset]
	offset++

	// Bots
	if offset >= len(data) {
		return nil, fmt.Errorf("missing bots")
	}
	info.Bots = data[offset]
	offset++

	// Server type
	if offset >= len(data) {
		return nil, fmt.Errorf("missing server type")
	}
	info.ServerType = data[offset]
	offset++

	// Environment
	if offset >= len(data) {
		return nil, fmt.Errorf("missing environment")
	}
	info.Environment = data[offset]
	offset++

	// Visibility
	if offset >= len(data) {
		return nil, fmt.Errorf("missing visibility")
	}
	info.Visibility = data[offset]
	offset++

	// VAC
	if offset >= len(data) {
		return nil, fmt.Errorf("missing VAC")
	}
	info.VAC = data[offset]
	offset++

	// Version
	version, newOffset, err := s.readNullTerminatedString(data, offset)
	if err != nil {
		return nil, fmt.Errorf("read version failed: %w", err)
	}
	info.Version = version

	return info, nil
}

func (s *SourceProtocol) parsePlayersResponse(data []byte) ([]Player, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("data too short")
	}

	playerCount := data[0]
	players := make([]Player, 0, playerCount)
	offset := 1

	for i := 0; i < int(playerCount); i++ {
		if offset >= len(data) {
			break
		}

		// Index (1 byte)
		offset++

		// Name
		name, newOffset, err := s.readNullTerminatedString(data, offset)
		if err != nil {
			break
		}
		offset = newOffset

		// Score (4 bytes)
		if offset+3 >= len(data) {
			break
		}
		score := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		offset += 4

		// Duration (4 bytes float)
		if offset+3 >= len(data) {
			break
		}
		durationFloat := binary.LittleEndian.Uint32(data[offset : offset+4])
		duration := time.Duration(durationFloat) * time.Second
		offset += 4

		players = append(players, Player{
			Name:     name,
			Score:    score,
			Duration: duration,
		})
	}

	return players, nil
}

func (s *SourceProtocol) readNullTerminatedString(data []byte, offset int) (string, int, error) {
	start := offset
	for offset < len(data) && data[offset] != 0 {
		offset++
	}
	if offset >= len(data) {
		return "", offset, fmt.Errorf("unterminated string")
	}
	return string(data[start:offset]), offset + 1, nil
}

// detectGameType determines the specific game based on game description and App ID
func (s *SourceProtocol) detectGameType(gameDesc string, appID uint16) string {
	gameLower := strings.ToLower(gameDesc)
	
	// Check by App ID first (most reliable)
	// Note: Some newer games have App IDs that exceed uint16 range
	switch appID {
	case 730:
		return "counter-strike"
	case 240: 
		return "counter-source"
	case 4000:
		return "garrys-mod"
	case 440:
		return "team-fortress-2"
	case 550:
		return "left-4-dead-2"
	case 500:
		return "left-4-dead"
	case 320:
		return "half-life"
	case 300:
		return "day-of-defeat-source"
	// Note: Rust (252490), Ark (346110), Insurgency (222880), Project Zomboid (108600)
	// have App IDs that exceed uint16 range - handled by string matching below
	}
	
	// Fallback to string matching
	if strings.Contains(gameLower, "counter-strike 2") {
		return "counter-strike-2"
	} else if strings.Contains(gameLower, "counter-strike: global offensive") {
		return "counter-strike"
	} else if strings.Contains(gameLower, "counter-strike") {
		return "counter-strike"
	} else if strings.Contains(gameLower, "garrysmod") || strings.Contains(gameLower, "garry") {
		return "garrys-mod"
	} else if strings.Contains(gameLower, "team fortress") {
		return "team-fortress-2"
	} else if strings.Contains(gameLower, "left 4 dead 2") {
		return "left-4-dead-2"
	} else if strings.Contains(gameLower, "left 4 dead") {
		return "left-4-dead"
	} else if strings.Contains(gameLower, "rust") {
		return "rust"
	} else if strings.Contains(gameLower, "ark") {
		return "ark-survival-evolved"
	} else if strings.Contains(gameLower, "insurgency") {
		return "insurgency"
	} else if strings.Contains(gameLower, "day of defeat") {
		return "day-of-defeat"
	} else if strings.Contains(gameLower, "project zomboid") {
		return "project-zomboid"
	} else if strings.Contains(gameLower, "satisfactory") {
		return "satisfactory"
	} else if strings.Contains(gameLower, "7 days to die") {
		return "7-days-to-die"
	}
	
	return "source"
}

// A2SInfo represents the parsed A2S_INFO response
type A2SInfo struct {
	Protocol    uint8
	Name        string
	Map         string
	Folder      string
	Game        string
	AppID       uint16
	Players     uint8
	MaxPlayers  uint8
	Bots        uint8
	ServerType  uint8
	Environment uint8
	Visibility  uint8
	VAC         uint8
	Version     string
}
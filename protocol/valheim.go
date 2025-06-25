package protocol

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"time"
)

// ValheimProtocol implements the Steam Query protocol for Valheim
type ValheimProtocol struct{}

func init() {
	registry.Register(&ValheimProtocol{})
}

func (v *ValheimProtocol) Name() string {
	return "valheim"
}

func (v *ValheimProtocol) DefaultPort() int {
	return 2456
}

func (v *ValheimProtocol) Query(ctx context.Context, addr string, opts *Options) (*ServerInfo, error) {
	// Valheim uses Steam query protocol on game port + 1
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("invalid address: %w", err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("invalid port: %w", err)
	}

	// Steam query port is game port + 1
	queryPort := port + 1
	queryAddr := net.JoinHostPort(host, strconv.Itoa(queryPort))

	conn, err := net.Dial("udp", queryAddr)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	// Set deadline
	conn.SetDeadline(time.Now().Add(opts.Timeout))

	start := time.Now()

	// Build Steam A2S_INFO request
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
		return v.queryWithChallenge(conn, challenge, opts.Timeout, start, opts)
	}

	// Check for A2S_INFO response
	if response[4] != 0x49 {
		return &ServerInfo{Online: false}, fmt.Errorf("unexpected response type: %02x", response[4])
	}

	// Parse A2S_INFO response
	info, err := v.parseA2SInfoResponse(response[5:n])
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

	// Query players if requested
	if opts.Players {
		players, err := v.queryPlayers(conn, opts.Timeout)
		if err == nil {
			result.Players.List = players
		} else {
			result.Players.List = make([]Player, 0)
		}
	}

	return result, nil
}

func (v *ValheimProtocol) queryWithChallenge(conn net.Conn, challenge uint32, timeout time.Duration, start time.Time, opts *Options) (*ServerInfo, error) {
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
	info, err := v.parseA2SInfoResponse(response[5:n])
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

	// Query players if requested
	if opts.Players {
		players, err := v.queryPlayers(conn, timeout)
		if err == nil {
			result.Players.List = players
		} else {
			result.Players.List = make([]Player, 0)
		}
	}

	return result, nil
}

func (v *ValheimProtocol) queryPlayers(conn net.Conn, timeout time.Duration) ([]Player, error) {
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

	return v.parsePlayersResponse(response[5:n])
}

func (v *ValheimProtocol) parseA2SInfoResponse(data []byte) (*ValheimInfo, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("data too short")
	}

	info := &ValheimInfo{}
	offset := 0

	// Protocol version
	if offset >= len(data) {
		return nil, fmt.Errorf("missing protocol version")
	}
	info.Protocol = data[offset]
	offset++

	// Name
	name, newOffset, err := v.readNullTerminatedString(data, offset)
	if err != nil {
		return nil, fmt.Errorf("read name failed: %w", err)
	}
	info.Name = name
	offset = newOffset

	// Map
	mapName, newOffset, err := v.readNullTerminatedString(data, offset)
	if err != nil {
		return nil, fmt.Errorf("read map failed: %w", err)
	}
	info.Map = mapName
	offset = newOffset

	// Folder
	folder, newOffset, err := v.readNullTerminatedString(data, offset)
	if err != nil {
		return nil, fmt.Errorf("read folder failed: %w", err)
	}
	info.Folder = folder
	offset = newOffset

	// Game
	game, newOffset, err := v.readNullTerminatedString(data, offset)
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
	version, newOffset, err := v.readNullTerminatedString(data, offset)
	if err != nil {
		return nil, fmt.Errorf("read version failed: %w", err)
	}
	info.Version = version

	return info, nil
}

func (v *ValheimProtocol) parsePlayersResponse(data []byte) ([]Player, error) {
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
		name, newOffset, err := v.readNullTerminatedString(data, offset)
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

func (v *ValheimProtocol) readNullTerminatedString(data []byte, offset int) (string, int, error) {
	start := offset
	for offset < len(data) && data[offset] != 0 {
		offset++
	}
	if offset >= len(data) {
		return "", offset, fmt.Errorf("unterminated string")
	}
	return string(data[start:offset]), offset + 1, nil
}

// ValheimInfo represents the parsed A2S_INFO response for Valheim
type ValheimInfo struct {
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
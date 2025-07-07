package protocol

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"time"
)

// A2SProtocol implements the A2S_INFO protocol
type A2SProtocol struct{}

func init() {
	registry.Register(&A2SProtocol{})
}

func (s *A2SProtocol) Name() string {
	return "a2s"
}

func (s *A2SProtocol) DefaultPort() int {
	return 27015
}

func (s *A2SProtocol) DefaultQueryPort() int {
	return 27015
}

func (s *A2SProtocol) Games() []GameConfig {
	return []GameConfig{
		// Standard A2S games using 27015
		{Name: "counter-strike-2", GamePort: 27015, QueryPort: 27015},
		{Name: "counter-strike", GamePort: 27015, QueryPort: 27015},
		{Name: "counter-source", GamePort: 27015, QueryPort: 27015},
		{Name: "garrys-mod", GamePort: 27015, QueryPort: 27015},
		{Name: "team-fortress-2", GamePort: 27015, QueryPort: 27015},
		{Name: "left-4-dead", GamePort: 27015, QueryPort: 27015},
		{Name: "left-4-dead-2", GamePort: 27015, QueryPort: 27015},
		{Name: "half-life", GamePort: 27015, QueryPort: 27015},
		{Name: "insurgency", GamePort: 27015, QueryPort: 27015},
		{Name: "day-of-defeat", GamePort: 27015, QueryPort: 27015},
		{Name: "project-zomboid", GamePort: 16261, QueryPort: 16261},
		{Name: "satisfactory", GamePort: 7777, QueryPort: 15777},
		{Name: "7-days-to-die", GamePort: 26900, QueryPort: 26900},
		{Name: "arma-3", GamePort: 2302, QueryPort: 2303},
		{Name: "dayz", GamePort: 2302, QueryPort: 27016},
		{Name: "battalion-1944", GamePort: 7777, QueryPort: 7777},

		// Games with non standard ports
		{Name: "rust", GamePort: 28015, QueryPort: 28015},
		{Name: "valheim", GamePort: 2456, QueryPort: 2457},
		{Name: "ark-survival-evolved", GamePort: 7777, QueryPort: 27015},
	}
}

func (s *A2SProtocol) DetectGame(info *ServerInfo) string {
	if info == nil || !info.Online {
		return "a2s"
	}

	// Use App ID for reliable game detection
	if info.Extra != nil {
		if appIDStr, exists := info.Extra["app_id"]; exists {
			if game := s.detectByAppID(appIDStr); game != "" {
				return game
			}
		}
	}
	
	// Default to generic a2s if no App ID or unknown App ID
	return "a2s"
}

func (s *A2SProtocol) Query(ctx context.Context, addr string, opts *Options) (*ServerInfo, error) {
	if opts.Debug {
		debugLogf("A2S", "Starting query for %s", addr)
	}

	conn, err := setupConnection(ctx, "udp", addr, opts)
	if err != nil {
		return &ServerInfo{Online: false}, err
	}
	defer conn.Close()

	// Build A2S_INFO request
	request := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x54}
	request = append(request, []byte("Source Engine Query\x00")...)

	if opts.Debug {
		debugLogf("A2S", "Sending A2S_INFO request (%d bytes)", len(request))
	}

	// Measure ping from request send to response receive
	pingStart := time.Now()

	// Send request
	if _, err := conn.Write(request); err != nil {
		if opts.Debug {
			debugLogf("A2S", "Request write failed: %v", err)
		}
		return &ServerInfo{Online: false}, fmt.Errorf("write failed: %w", err)
	}

	// Read response
	response := make([]byte, 1400)
	n, err := conn.Read(response)
	pingDuration := time.Since(pingStart)
	ping := int(math.Ceil(float64(pingDuration.Nanoseconds()) / 1e6))

	if err != nil {
		if opts.Debug {
			debugLogf("A2S", "Response read failed: %v", err)
		}
		return &ServerInfo{Online: false}, fmt.Errorf("read failed: %w", err)
	}

	if opts.Debug {
		debugLogf("A2S", "Received %d bytes response (ping: %dms)", n, ping)
	}

	if n < 5 {
		if opts.Debug {
			debugLogf("A2S", "Response too short (%d bytes)", n)
		}
		return &ServerInfo{Online: false}, fmt.Errorf("response too short")
	}

	// Check for challenge response
	if response[4] == 0x41 { // Challenge response
		if opts.Debug {
			debugLog("A2S", "Received challenge response")
		}
		if n < 9 {
			return &ServerInfo{Online: false}, fmt.Errorf("challenge response too short")
		}
		challenge := binary.LittleEndian.Uint32(response[5:9])
		if opts.Debug {
			debugLogf("A2S", "Challenge value: 0x%08x", challenge)
		}
		return s.queryWithChallenge(conn, addr, challenge, getTimeout(opts), ping, opts)
	}

	// Check for A2S_INFO response
	if response[4] != 0x49 {
		if opts.Debug {
			debugLogf("A2S", "Unexpected response type: 0x%02x (expected 0x49)", response[4])
		}
		return &ServerInfo{Online: false}, fmt.Errorf("unexpected response type: %02x", response[4])
	}

	if opts.Debug {
		debugLog("A2S", "Parsing A2S_INFO response")
	}

	// Parse A2S_INFO response
	info, err := s.parseA2SInfoResponse(response[5:n])
	if err != nil {
		if opts.Debug {
			debugLogf("A2S", "Response parsing failed: %v", err)
		}
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
		// Store game description and App ID for central game detector
		Extra: map[string]string{
			"game":   info.Game,
			"app_id": fmt.Sprintf("%d", info.AppID),
		},
	}

	if opts.Debug {
		debugLogf("A2S", "Parsed server info - Name: '%s', Game: '%s', Map: '%s', Players: %d/%d",
			result.Name, info.Game, result.Map, result.Players.Current, result.Players.Max)
	}

	// Use protocol-specific game detection
	result.Game = s.DetectGame(result)

	if opts.Debug {
		debugLogf("A2S", "Detected game type: '%s'", result.Game)
	}

	// Query players if requested
	if opts.Players {
		if opts.Debug {
			debugLog("A2S", "Querying player list")
		}
		players, err := s.queryPlayers(conn, addr, getTimeout(opts))
		if err == nil {
			result.Players.List = players
			if opts.Debug {
				debugLogf("A2S", "Retrieved %d players", len(players))
			}
		} else {
			if opts.Debug {
				debugLogf("A2S", "Player query failed: %v", err)
			}
			result.Players.List = make([]Player, 0)
		}
	}

	if opts.Debug {
		debugLog("A2S", "Query completed successfully")
	}
	return result, nil
}

func (s *A2SProtocol) queryWithChallenge(conn net.Conn, addr string, challenge uint32, timeout time.Duration, initialPing int, opts *Options) (*ServerInfo, error) {
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

	// Use the initial ping from the first request rather than measuring challenge exchange
	ping := initialPing

	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("read challenge response failed: %w", err)
	}

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
		// Store game description and App ID for central game detector
		Extra: map[string]string{
			"game":   info.Game,
			"app_id": fmt.Sprintf("%d", info.AppID),
		},
	}

	// Use protocol-specific game detection
	result.Game = s.DetectGame(result)

	// Query players if requested
	if opts.Players {
		players, err := s.queryPlayers(conn, addr, getTimeout(opts))
		if err == nil {
			result.Players.List = players
		} else {
			result.Players.List = make([]Player, 0)
		}
	}

	return result, nil
}

func (s *A2SProtocol) queryPlayers(conn net.Conn, addr string, timeout time.Duration) ([]Player, error) {
	// A2S_PLAYER request
	request := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x55}
	challengeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(challengeBytes, 0xFFFFFFFF)
	request = append(request, challengeBytes...)

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

func (s *A2SProtocol) parseA2SInfoResponse(data []byte) (*A2SInfo, error) {
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

func (s *A2SProtocol) parsePlayersResponse(data []byte) ([]Player, error) {
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
		durationBits := binary.LittleEndian.Uint32(data[offset : offset+4])
		durationFloat := math.Float32frombits(durationBits)
		// Round to nearest second
		duration := time.Duration(math.Round(float64(durationFloat))) * time.Second
		offset += 4

		players = append(players, Player{
			Name:     name,
			Score:    score,
			Duration: duration,
		})
	}

	return players, nil
}

func (s *A2SProtocol) readNullTerminatedString(data []byte, offset int) (string, int, error) {
	start := offset
	for offset < len(data) && data[offset] != 0 {
		offset++
	}
	if offset >= len(data) {
		return "", offset, fmt.Errorf("unterminated string")
	}
	return string(data[start:offset]), offset + 1, nil
}

// detectGameType has been moved to central game detector in gamedetector.go

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

// detectByAppID determines game type from Steam App ID
func (s *A2SProtocol) detectByAppID(appIDStr string) string {
	// Convert string to int for comparison
	var appID int
	if _, err := fmt.Sscanf(appIDStr, "%d", &appID); err != nil {
		return ""
	}
	
	// Check by App ID first (most reliable)
	switch appID {
	case 730:
		return "counter-strike"
	case 240:
		return "counter-strike"
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
		return "day-of-defeat"
	case 252490:
		return "rust"
	case 346110:
		return "ark-survival-evolved"
	case 222880:
		return "insurgency"
	case 108600:
		return "project-zomboid"
	case 526870:
		return "satisfactory"
	case 251570:
		return "7-days-to-die"
	case 892970:
		return "valheim"
	case 107410:
		return "arma-3"
	case 221100:
		return "dayz"
	case 489940:
		return "battalion-1944"
	}
	
	return ""
}


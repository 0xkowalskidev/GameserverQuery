package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"
)

// FactorioProtocol implements the Factorio UDP query protocol
type FactorioProtocol struct{}

func init() {
	registry.Register(&FactorioProtocol{})
}

func (f *FactorioProtocol) Name() string {
	return "factorio"
}

func (f *FactorioProtocol) DefaultPort() int {
	return 34197
}

func (f *FactorioProtocol) Query(ctx context.Context, addr string, opts *Options) (*ServerInfo, error) {
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	// Set deadline
	conn.SetDeadline(time.Now().Add(opts.Timeout))

	start := time.Now()

	// Factorio server query packet
	// The packet format is: {0x00, 0x00, 0x00, 0x00, 0x00, 0x01}
	request := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01}

	// Send request
	if _, err := conn.Write(request); err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("write failed: %w", err)
	}

	// Read response
	response := make([]byte, 1024)
	n, err := conn.Read(response)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("read failed: %w", err)
	}

	ping := int(time.Since(start).Nanoseconds() / 1e6)

	if n < 6 {
		return &ServerInfo{Online: false}, fmt.Errorf("response too short")
	}

	// Parse response
	info, err := f.parseResponse(response[:n])
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("parse failed: %w", err)
	}

	info.Ping = ping
	return info, nil
}

func (f *FactorioProtocol) parseResponse(data []byte) (*ServerInfo, error) {
	// Skip the response header (6 bytes)
	if len(data) < 6 {
		return nil, fmt.Errorf("response too short")
	}

	// The response format varies, but typically contains JSON data
	// Try to find JSON start
	jsonStart := -1
	for i := 6; i < len(data); i++ {
		if data[i] == '{' {
			jsonStart = i
			break
		}
	}

	if jsonStart == -1 {
		// No JSON found, create basic response
		return &ServerInfo{
			Name:    "Factorio Server",
			Game:    f.Name(),
			Version: "Unknown",
			Online:  true,
			Players: PlayerInfo{
				Current: 0,
				Max:     0,
				List:    make([]Player, 0),
			},
		}, nil
	}

	// Try to parse JSON
	jsonData := data[jsonStart:]
	
	// Find the end of JSON (naive approach)
	jsonEnd := len(jsonData)
	for i := len(jsonData) - 1; i >= 0; i-- {
		if jsonData[i] == '}' {
			jsonEnd = i + 1
			break
		}
	}

	var factorioInfo FactorioServerInfo
	if err := json.Unmarshal(jsonData[:jsonEnd], &factorioInfo); err != nil {
		// JSON parsing failed, return basic info
		return &ServerInfo{
			Name:    "Factorio Server",
			Game:    f.Name(),
			Version: "Unknown",
			Online:  true,
			Players: PlayerInfo{
				Current: 0,
				Max:     0,
				List:    make([]Player, 0),
			},
		}, nil
	}

	players := make([]Player, 0, len(factorioInfo.Players))
	for _, playerName := range factorioInfo.Players {
		players = append(players, Player{Name: playerName})
	}

	return &ServerInfo{
		Name:    factorioInfo.Name,
		Game:    f.Name(),
		Version: factorioInfo.Version,
		Online:  true,
		Players: PlayerInfo{
			Current: len(factorioInfo.Players),
			Max:     factorioInfo.MaxPlayers,
			List:    players,
		},
		Extra: map[string]string{
			"game_time": strconv.Itoa(factorioInfo.GameTime),
			"has_mods":  strconv.FormatBool(factorioInfo.HasMods),
		},
	}, nil
}

// FactorioServerInfo represents Factorio server response
type FactorioServerInfo struct {
	Name       string   `json:"name"`
	Version    string   `json:"application_version"`
	GameTime   int      `json:"game_time_elapsed"`
	MaxPlayers int      `json:"max_players"`
	Players    []string `json:"players"`
	HasMods    bool     `json:"has_mods"`
}
package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TerrariaProtocol implements the Terraria native protocol
type TerrariaProtocol struct{}

func init() {
	registry.Register(&TerrariaProtocol{})
}

func (t *TerrariaProtocol) Name() string {
	return "terraria"
}

func (t *TerrariaProtocol) DefaultPort() int {
	return 7777
}

func (t *TerrariaProtocol) Query(ctx context.Context, addr string, opts *Options) (*ServerInfo, error) {
	conn, err := setupConnection(ctx, "tcp", addr, opts)
	if err != nil {
		return &ServerInfo{Online: false}, err
	}
	defer conn.Close()

	start := time.Now()

	// Try TShock REST API first (more reliable)
	if info, err := t.queryTShockAPI(ctx, addr, getTimeout(opts)); err == nil {
		info.Ping = int(time.Since(start).Nanoseconds() / 1e6)
		return info, nil
	}

	// Fallback to native protocol
	// Send server info request packet
	serverInfoPacket := []byte{
		0x05, 0x00, 0x00, 0x00, // Length: 5 bytes (excluding length field)
		0x01, // Packet type: Server Info Request
	}

	if _, err := conn.Write(serverInfoPacket); err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("write server info request failed: %w", err)
	}

	// Read response - could be any packet type
	response := make([]byte, 1024)
	n, err := conn.Read(response)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("read failed: %w", err)
	}

	ping := int(time.Since(start).Nanoseconds() / 1e6)

	// Parse whatever response we get
	info, err := t.parseResponse(response[:n])
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("parse failed: %w", err)
	}

	info.Ping = ping
	return info, nil
}

func (t *TerrariaProtocol) parseResponse(data []byte) (*ServerInfo, error) {
	if len(data) < 5 {
		return nil, fmt.Errorf("response too short")
	}

	// Skip packet length (4 bytes)
	offset := 4
	
	// Check packet type and handle accordingly
	packetType := data[offset]
	offset++

	// Handle different packet types - any valid response means server is online
	switch packetType {
	case 0x13: // Player Info packet
		// This is common when a server responds with player info
		// Just having a valid response means the server is online
		info := &ServerInfo{
			Name:    "Terraria Server (Player Info)",
			Game:    t.Name(),
			Version: "Unknown",
			Online:  true,
			Players: PlayerInfo{
				Current: 0,
				Max:     8,
				List:    make([]Player, 0),
			},
		}
		return info, nil
		
	case 0x19: // Chat message response
		// Continue with original parsing logic
		break
		
	default:
		// Any valid packet response means the server is a Terraria server
		info := &ServerInfo{
			Name:    fmt.Sprintf("Terraria Server (Type: 0x%02x)", packetType),
			Game:    t.Name(),
			Version: "Unknown", 
			Online:  true,
			Players: PlayerInfo{
				Current: 0,
				Max:     8,
				List:    make([]Player, 0),
			},
		}
		return info, nil
	}

	// Skip player ID
	if offset >= len(data) {
		return nil, fmt.Errorf("missing player ID")
	}
	offset++

	// Read text length
	if offset >= len(data) {
		return nil, fmt.Errorf("missing text length")
	}
	textLength := int(data[offset])
	offset++

	// Read text
	if offset+textLength > len(data) {
		return nil, fmt.Errorf("text length exceeds data")
	}
	text := string(data[offset : offset+textLength])

	// Parse the response text to extract server information
	info := &ServerInfo{
		Name:    "Terraria Server",
		Game:    t.Name(),
		Version: "Unknown",
		Online:  true,
		Players: PlayerInfo{
			Current: 0,
			Max:     8, // Default Terraria max
		},
	}

	// Try to extract player count from common response patterns
	// Pattern 1: "Online players: X/Y"
	playerPattern := regexp.MustCompile(`Online players?:\s*(\d+)/?(\d+)?`)
	if matches := playerPattern.FindStringSubmatch(text); len(matches) >= 2 {
		if current, err := strconv.Atoi(matches[1]); err == nil {
			info.Players.Current = current
		}
		if len(matches) >= 3 && matches[2] != "" {
			if max, err := strconv.Atoi(matches[2]); err == nil {
				info.Players.Max = max
			}
		}
	}

	// Pattern 2: "Players online: X"
	playerPattern2 := regexp.MustCompile(`Players? online:\s*(\d+)`)
	if matches := playerPattern2.FindStringSubmatch(text); len(matches) >= 2 {
		if current, err := strconv.Atoi(matches[1]); err == nil {
			info.Players.Current = current
		}
	}

	// Pattern 3: "X players currently online"
	playerPattern3 := regexp.MustCompile(`(\d+)\s+players? currently online`)
	if matches := playerPattern3.FindStringSubmatch(text); len(matches) >= 2 {
		if current, err := strconv.Atoi(matches[1]); err == nil {
			info.Players.Current = current
		}
	}

	// Initialize Players.List to empty slice
	info.Players.List = make([]Player, 0)

	// Try to extract player names if present
	if strings.Contains(text, ":") {
		parts := strings.Split(text, ":")
		if len(parts) > 1 {
			playerList := strings.TrimSpace(parts[1])
			if playerList != "" && playerList != "None" && !strings.Contains(playerList, "No players") {
				players := strings.Split(playerList, ",")
				info.Players.List = make([]Player, 0, len(players))
				for _, playerName := range players {
					name := strings.TrimSpace(playerName)
					if name != "" {
						info.Players.List = append(info.Players.List, Player{Name: name})
					}
				}
				// Update current count based on actual player list
				if len(info.Players.List) > 0 {
					info.Players.Current = len(info.Players.List)
				}
			}
		}
	}

	return info, nil
}

// queryTShockAPI attempts to query TShock REST API
func (t *TerrariaProtocol) queryTShockAPI(ctx context.Context, addr string, timeout time.Duration) (*ServerInfo, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	_, err = strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %w", err)
	}

	// TShock REST API is typically on port 7878
	restPort := 7878
	
	// Try common TShock REST API endpoints
	endpoints := []string{
		fmt.Sprintf("http://%s:%d/v2/server/status", host, restPort),
		fmt.Sprintf("http://%s:%d/status", host, restPort),
		fmt.Sprintf("http://%s:%d/v3/server/status", host, restPort),
	}

	client := &http.Client{Timeout: timeout}

	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var tshockStatus TShockStatus
			if err := json.NewDecoder(resp.Body).Decode(&tshockStatus); err != nil {
				continue
			}

			return &ServerInfo{
				Name:    tshockStatus.Name,
				Version: tshockStatus.TerrariaVersion,
				Online:  true,
				Players: PlayerInfo{
					Current: tshockStatus.PlayerCount,
					Max:     tshockStatus.MaxPlayers,
					List:    make([]Player, 0),
				},
				Game: "terraria",
				Extra: map[string]string{
					"world":      tshockStatus.World,
					"tshock":     tshockStatus.TShockVersion,
					"difficulty": strconv.Itoa(tshockStatus.Difficulty),
				},
			}, nil
		}
	}

	return nil, fmt.Errorf("TShock API not available")
}

// TShockStatus represents TShock REST API response
type TShockStatus struct {
	Name            string `json:"name"`
	World           string `json:"world"`
	PlayerCount     int    `json:"playercount"`
	MaxPlayers      int    `json:"maxplayers"`
	TerrariaVersion string `json:"terraria_version"`
	TShockVersion   string `json:"tshock_version"`
	Difficulty      int    `json:"difficulty"`
}
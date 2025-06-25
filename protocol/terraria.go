package protocol

import (
	"context"
	"fmt"
	"net"
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
	// Use context for connection timeout
	dialer := &net.Dialer{Timeout: opts.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	// Set deadline based on context or timeout
	deadline := time.Now().Add(opts.Timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	conn.SetDeadline(deadline)

	// Send a simple connection request packet first
	// Packet 1: Connect Request (0x01)
	connectPacket := []byte{
		0x05, 0x00, 0x00, 0x00, // Length: 5 bytes (excluding length field)
		0x01, // Packet type: Connect Request
	}

	if _, err := conn.Write(connectPacket); err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("write connect failed: %w", err)
	}

	// Read response - could be any packet type
	response := make([]byte, 1024)
	n, err := conn.Read(response)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("read failed: %w", err)
	}

	// Parse whatever response we get
	info, err := t.parseResponse(response[:n])
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("parse failed: %w", err)
	}

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
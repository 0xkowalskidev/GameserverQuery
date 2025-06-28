package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MinecraftProtocol implements the Minecraft Server List Ping protocol
type MinecraftProtocol struct{}

func init() {
	registry.Register(&MinecraftProtocol{})
}

func (m *MinecraftProtocol) Name() string {
	return "minecraft"
}

func (m *MinecraftProtocol) DefaultPort() int {
	return 25565
}

func (m *MinecraftProtocol) Query(ctx context.Context, addr string, opts *Options) (*ServerInfo, error) {
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

	// Parse host and port for handshake
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("invalid address: %w", err)
	}
	
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("invalid port: %w", err)
	}

	// Send handshake packet
	if err := m.sendHandshake(conn, host, port); err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("handshake failed: %w", err)
	}

	// Send status request
	if err := m.sendStatusRequest(conn); err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("status request failed: %w", err)
	}

	// Read response
	responseData, err := m.readVarIntPrefixedData(conn)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("read response failed: %w", err)
	}

	// Skip packet ID
	if len(responseData) < 1 {
		return &ServerInfo{Online: false}, fmt.Errorf("response too short")
	}
	
	// Read JSON string length and data
	reader := bytes.NewReader(responseData[1:])
	jsonLength, err := m.readVarInt(reader)
	if err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("read JSON length failed: %w", err)
	}
	
	jsonData := make([]byte, jsonLength)
	if _, err := io.ReadFull(reader, jsonData); err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("read JSON data failed: %w", err)
	}

	// Parse JSON response
	var status MinecraftStatus
	if err := json.Unmarshal(jsonData, &status); err != nil {
		return &ServerInfo{Online: false}, fmt.Errorf("failed to parse JSON: %w", err)
	}

	motd := m.cleanMotd(status.Description)
	
	info := &ServerInfo{
		Name:    motd, // Use MOTD as the server name for Minecraft
		Game:    m.Name(),
		Version: status.Version.Name,
		Online:  true,
		Players: PlayerInfo{
			Current: status.Players.Online,
			Max:     status.Players.Max,
		},
	}

	// Add player list if requested
	if opts.Players {
		if status.Players.Sample != nil {
			info.Players.List = make([]Player, len(status.Players.Sample))
			for i, player := range status.Players.Sample {
				info.Players.List[i] = Player{Name: player.Name}
			}
		} else {
			info.Players.List = make([]Player, 0)
		}
	}

	return info, nil
}

func (m *MinecraftProtocol) sendHandshake(conn net.Conn, host string, port int) error {
	var buf bytes.Buffer
	
	// Protocol version (VarInt): use a modern version like 765 (1.20.4)
	m.writeVarInt(&buf, 765)
	
	// Server address (String)
	m.writeString(&buf, host)
	
	// Server port (Unsigned Short)
	buf.WriteByte(byte(port >> 8))
	buf.WriteByte(byte(port))
	
	// Next state (VarInt): 1 for status
	m.writeVarInt(&buf, 1)
	
	// Create packet with packet ID 0x00
	packet := bytes.Buffer{}
	m.writeVarInt(&packet, 0) // Packet ID
	packet.Write(buf.Bytes())
	
	// Send packet with length prefix
	return m.writeVarIntPrefixedData(conn, packet.Bytes())
}

func (m *MinecraftProtocol) sendStatusRequest(conn net.Conn) error {
	// Status request packet: just packet ID 0x00 with no data
	packet := []byte{0x00}
	return m.writeVarIntPrefixedData(conn, packet)
}

func (m *MinecraftProtocol) writeVarInt(buf *bytes.Buffer, value int) {
	for {
		if (value & 0xFFFFFF80) == 0 {
			buf.WriteByte(byte(value))
			break
		}
		buf.WriteByte(byte((value & 0x7F) | 0x80))
		value >>= 7
	}
}

func (m *MinecraftProtocol) writeString(buf *bytes.Buffer, str string) {
	data := []byte(str)
	m.writeVarInt(buf, len(data))
	buf.Write(data)
}

func (m *MinecraftProtocol) writeVarIntPrefixedData(conn net.Conn, data []byte) error {
	var buf bytes.Buffer
	m.writeVarInt(&buf, len(data))
	buf.Write(data)
	_, err := conn.Write(buf.Bytes())
	return err
}

func (m *MinecraftProtocol) readVarInt(reader io.Reader) (int, error) {
	var result int
	var shift uint
	
	for {
		var b [1]byte
		if _, err := io.ReadFull(reader, b[:]); err != nil {
			return 0, err
		}
		
		result |= int(b[0]&0x7F) << shift
		if (b[0] & 0x80) == 0 {
			break
		}
		shift += 7
		if shift >= 32 {
			return 0, fmt.Errorf("VarInt too long")
		}
	}
	
	return result, nil
}

func (m *MinecraftProtocol) readVarIntPrefixedData(reader io.Reader) ([]byte, error) {
	length, err := m.readVarInt(reader)
	if err != nil {
		return nil, err
	}
	
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}
	
	return data, nil
}

func (m *MinecraftProtocol) cleanMotd(motd interface{}) string {
	var text string
	
	switch v := motd.(type) {
	case string:
		text = v
	case map[string]interface{}:
		if textVal, ok := v["text"].(string); ok {
			text = textVal
		}
		if extra, ok := v["extra"].([]interface{}); ok {
			for _, item := range extra {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if itemText, ok := itemMap["text"].(string); ok {
						text += itemText
					}
				} else if itemStr, ok := item.(string); ok {
					text += itemStr
				}
			}
		}
	}
	
	// Remove Minecraft color codes and formatting
	colorCodeRe := regexp.MustCompile(`§[0-9a-fk-or]`)
	text = colorCodeRe.ReplaceAllString(text, "")
	
	return strings.TrimSpace(text)
}

// MinecraftStatus represents the JSON response from a Minecraft server
type MinecraftStatus struct {
	Version struct {
		Name     string `json:"name"`
		Protocol int    `json:"protocol"`
	} `json:"version"`
	Players struct {
		Max    int `json:"max"`
		Online int `json:"online"`
		Sample []struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"sample,omitempty"`
	} `json:"players"`
	Description interface{} `json:"description"`
	Favicon     string      `json:"favicon,omitempty"`
}
# GameserverQuery

## It does what it says on the tin.

This is a CLI tool and Go library for querying game servers across multiple games including Minecraft, Source engine games, Terraria, etc

## Features

- **Multi-game support**: Minecraft, Source games (CS2, CS:GO, Rust, Ark, etc.), Terraria, Valheim, 7 Days to Die, and more
- **Auto-detection**: Detects game type automatically
- **Player lists**: Get player information where supported
- **Server details**: Names, versions, maps where available
- **JSON/text output**: Multiple output formats
- **Zero dependencies**: Pure Go, no external deps

## Installation

```bash
go install github.com/0xkowalskidev/gameserverquery@latest
```

## CLI Usage

### Basic Query
```bash
# Auto-detect game type (slower then querying for a specific game)
gameserverquery localhost:25565

# Query specific game type
gameserverquery -game minecraft play.hypixel.net

# Query specific games
gameserverquery -game counter-strike-2 192.168.1.100:27015
gameserverquery -game rust 192.168.1.100:28015
gameserverquery -game ark-survival-evolved 192.168.1.100:27015

# Query with player list
gameserverquery -game minecraft -players play.hypixel.net

# JSON output
gameserverquery -game minecraft -format json play.hypixel.net
```

### Available Options
```bash
# List supported games
gameserverquery -list

# Custom timeout
gameserverquery -timeout 10s localhost:25565
```

### Supported Games

Run `gameserverquery -list` to see all supported games. Popular ones include:

**Core Protocols:**
- `minecraft` - Minecraft Server List Ping
- `source` - Source/Steam Query protocol (auto-detects specific games)
- `terraria` - Terraria native protocol with TShock REST API support

**Source/Steam Query Games:**
- `counter-strike-2` `counter-strike` `counter-source` `garrys-mod` `team-fortress-2`
- `rust` `ark-survival-evolved` `left-4-dead` `left-4-dead-2` `half-life`
- `insurgency` `day-of-defeat` `project-zomboid` `valheim` `satisfactory` `7-days-to-die`

## Library Usage

Add to your `go.mod`:
```go
require github.com/0xkowalskidev/gameserverquery v0.1.0
```

```go
import "github.com/0xkowalskidev/gameserverquery/query"

// Query a specific game
info, err := query.Query(ctx, "minecraft", "play.hypixel.net:25565")

// Auto-detect game type (slower, more resource intensive)
info, err := query.AutoDetect(ctx, "localhost:25565")

// With player list
info, err := query.Query(ctx, "counter-strike-2", "server.com:27015", query.WithPlayers())

// Query other games
info, err := query.Query(ctx, "rust", "rust-server.com:28015")
info, err := query.Query(ctx, "terraria", "terraria.example.com:7777")
info, err := query.Query(ctx, "terraria", "terraria.example.com:7777")
```

## Server Info Structure

The query functions return a `ServerInfo` struct with the following fields:

```go
type ServerInfo struct {
    Name        string            `json:"name"`         // Server name
    Game        string            `json:"game"`         // Game type identifier 
    Version     string            `json:"version"`      // Game/server version
    Address     string            `json:"address"`      // Server address
    Port        int               `json:"port"`         // Requested server port
    QueryPort   int               `json:"query_port"`   // Actual port that responded
    Players     PlayerInfo        `json:"players"`      // Player information
    Map         string            `json:"map,omitempty"`         // Current map (optional)
    Ping        time.Duration     `json:"ping"`         // Query response time
    Online      bool              `json:"online"`       // Server online status
    Extra       map[string]string `json:"extra,omitempty"`       // Additional game-specific data
}

type PlayerInfo struct {
    Current int      `json:"current"`           // Current player count
    Max     int      `json:"max"`              // Maximum player count
    List    []Player `json:"list,omitempty"`   // List of individual players (optional)
}

type Player struct {
    Name     string        `json:"name"`                    // Player name
    Score    int           `json:"score,omitempty"`         // Player score (optional)
    Duration time.Duration `json:"duration,omitempty"`      // Time played (optional)
}
```

## License

MIT License - see LICENSE file for details.

# GameserverQuery

## It does what it says on the tin.

This is a CLI tool and Go library for querying game servers across multiple games including Minecraft, Source engine games, Terraria, etc

## Features

- **Multi-game support**: Minecraft, Source games (CS2, CS:GO, Rust, Ark, etc.), Terraria, Valheim, Factorio, 7 Days to Die, and more
- **Auto-detection**: Detects game type automatically
- **Player lists**: Get player information where supported
- **Server details**: Names, versions, maps, MOTDs where available
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
gameserverquery -game counterstrike2 192.168.1.100:27015
gameserverquery -game rust 192.168.1.100:28015
gameserverquery -game arksurvivalevolved 192.168.1.100:27015

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

# Help
gameserverquery -help
```

### Supported Games

Run `gameserverquery -list` to see all supported games. Popular ones include:

**Core Protocols:**
- `minecraft` - Minecraft Server List Ping
- `source` - Source/Steam Query protocol (auto-detects specific games)
- `terraria` - Terraria native protocol with TShock REST API support
- `factorio` - Factorio UDP Query protocol

**Source/Steam Query Games:**
- `counterstrike2` `counterstrike` `countersource` `garrysmod` `teamfortress2`
- `rust` `arksurvivalevolved` `left4dead` `left4dead2` `halflife`
- `insurgency` `dayofdefeat` `projectzomboid` `valheim` `satisfactory` `7daystodie`

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
info, err := query.Query(ctx, "counterstrike2", "server.com:27015", query.WithPlayers())

// Query other games
info, err := query.Query(ctx, "rust", "rust-server.com:28015")
info, err := query.Query(ctx, "factorio", "factorio.example.com:34197")
info, err := query.Query(ctx, "terraria", "terraria.example.com:7777")
```

## API

- `query.Query(ctx, game, addr, ...opts)` - Query specific game
- `query.AutoDetect(ctx, addr, ...opts)` - Auto-detect game type  
- `query.WithPlayers()` - Include player list option

## License

MIT License - see LICENSE file for details.

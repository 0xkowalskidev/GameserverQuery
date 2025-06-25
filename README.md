# GameserverQuery

## It does what it says on the tin.

This is CLI tool and Go library for querying game servers across multiple games including Minecraft, Source engine games, Terraria, etc

## Features

- **Multi-game support**: Minecraft, CS2, CS:GO, Garry's Mod, Terraria, Valheim, etc.
- **Auto-detection**: Detects game type automatically
- **Player lists**: Get player information 
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

# Query specific Source games
gameserverquery -game cs2 192.168.1.100:27015
gameserverquery -game gmod 192.168.1.100:27015

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

`minecraft` `cs2` `csgo` `gmod` `tf2` `terraria` `valheim`

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
info, err := query.Query(ctx, "cs2", "server.com:27015", query.WithPlayers())
```

## API

- `query.Query(ctx, game, addr, ...opts)` - Query specific game
- `query.AutoDetect(ctx, addr, ...opts)` - Auto-detect game type  
- `query.WithPlayers()` - Include player list option

## License

MIT License - see LICENSE file for details.

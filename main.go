package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/0xkowalskidev/gameserverquery/protocol"
	"github.com/0xkowalskidev/gameserverquery/query"
	// Import all protocol implementations to register them
	_ "github.com/0xkowalskidev/gameserverquery/protocol"
)

func main() {
	var (
		timeout    = flag.Duration("timeout", 5*time.Second, "Query timeout")
		format     = flag.String("format", "text", "Output format (text, json)")
		players    = flag.Bool("players", false, "Include player list")
		game       = flag.String("game", "", "Game type (auto-detect if not specified)")
		showGames  = flag.Bool("list", false, "List supported games")
		help       = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *help {
		showHelp()
		return
	}

	if *showGames {
		listGames()
		return
	}

	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: gameserverquery [options] <address[:port]>\n")
		fmt.Fprintf(os.Stderr, "Run 'gameserverquery -help' for more information\n")
		os.Exit(1)
	}

	address := args[0]
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Build options
	var opts []query.Option
	if *players {
		opts = append(opts, query.WithPlayers())
	}

	var info *protocol.ServerInfo
	var err error

	if *game != "" {
		// Query specific game
		info, err = query.Query(ctx, *game, address, opts...)
	} else {
		// Auto-detect
		info, err = query.AutoDetect(ctx, address, opts...)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := outputResult(info, *format); err != nil {
		fmt.Fprintf(os.Stderr, "Output error: %v\n", err)
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Printf(`GameserverQuery - Query game servers for status information

Usage: gameserverquery [options] <address[:port]>

Options:
  -timeout duration    Query timeout (default 5s)
  -format string       Output format: text, json (default "text")  
  -players             Include player list
  -game string         Game type (minecraft, cs2, csgo, gmod, tf2, terraria, valheim, etc.)
  -list                List supported games
  -help                Show this help

Examples:
  gameserverquery localhost:25565                    # Auto-detect game type
  gameserverquery -game minecraft play.hypixel.net  # Query Minecraft server
  gameserverquery -game cs2 192.168.1.100:27015    # Query CS2 server  
  gameserverquery -game gmod 192.168.1.100:27015   # Query Garry's Mod server
  gameserverquery -players -format json localhost   # Include players, JSON output
  gameserverquery -list                             # Show supported games

Use -list to see all supported game names.

Auto-detection tries common game types based on port numbers or by testing protocols.
`)
}

func listGames() {
	games := query.SupportedGames()
	sort.Strings(games)

	fmt.Println("Supported games:")
	for _, game := range games {
		port := query.DefaultPort(game)
		fmt.Printf("  %-15s (default port: %d)\n", game, port)
	}
}

func outputResult(info *protocol.ServerInfo, format string) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(info)
	case "text":
		return outputText(info)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func outputText(info *protocol.ServerInfo) error {
	if !info.Online {
		fmt.Printf("Server %s:%d is offline\n", info.Address, info.Port)
		return nil
	}

	// Basic server info
	printIfNotEmpty("Server", info.Name)
	fmt.Printf("Game: %s\n", info.Game)
	if info.Version != "" {
		fmt.Printf("Version: %s\n", info.Version)
	}
	fmt.Printf("Address: %s:%d\n", info.Address, info.Port)
	fmt.Printf("Players: %d/%d\n", info.Players.Current, info.Players.Max)
	
	// Optional fields
	printIfNotEmpty("Map", info.Map)
	printIfNotEmpty("MOTD", info.MOTD)
	if info.Ping > 0 {
		fmt.Printf("Ping: %v\n", info.Ping)
	}
	fmt.Printf("Online: %t\n", info.Online)

	// Extra information
	printExtra(info.Extra)
	
	// Player list
	printPlayers(info.Players.List)

	return nil
}

func printIfNotEmpty(label, value string) {
	if value != "" {
		fmt.Printf("%s: %s\n", label, value)
	}
}

func printExtra(extra map[string]string) {
	if len(extra) > 0 {
		fmt.Println("\nExtra Information:")
		for key, value := range extra {
			fmt.Printf("  %s: %s\n", key, value)
		}
	}
}

func printPlayers(players []protocol.Player) {
	if len(players) > 0 {
		fmt.Println("\nPlayers:")
		for _, player := range players {
			parts := []string{player.Name}
			if player.Score > 0 {
				parts = append(parts, fmt.Sprintf("Score: %d", player.Score))
			}
			if player.Duration > 0 {
				parts = append(parts, fmt.Sprintf("Time: %v", player.Duration))
			}
			fmt.Printf("  %s\n", strings.Join(parts, " "))
		}
	}
}
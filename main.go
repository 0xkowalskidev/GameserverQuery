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
)

func main() {
	// Show help if no arguments provided
	if len(os.Args) < 2 {
		showHelp()
		return
	}

	// Default to query command if flag is specified
	if strings.HasPrefix(os.Args[1], "-") {
		// Run query command by default
		queryCmd()
		return
	}

	switch os.Args[1] {
	case "scan":
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
		scanCmd()
	case "list":
		listGames()
	default:
		queryCmd()
	}
}

func queryCmd() {
	var (
		timeout = flag.Duration("timeout", 5*time.Second, "Query timeout")
		format  = flag.String("format", "text", "Output format (text, json)")
		players = flag.Bool("players", false, "Include player list")
		game    = flag.String("game", "", "Game type (auto-detect if not specified)")
		debug   = flag.Bool("debug", false, "Enable debug logging")
	)
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: gameserverquery [query] [options] <address[:port]>\n")
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
	if *debug {
		opts = append(opts, query.WithDebug())
	}

	var info *protocol.ServerInfo
	var err error

	if *game != "" {
		// Query specific game
		opts = append(opts, query.WithGame(*game))
	}
	// Auto-detect if no game specified
	info, err = query.Query(ctx, address, opts...)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := outputResult(info, *format); err != nil {
		fmt.Fprintf(os.Stderr, "Output error: %v\n", err)
		os.Exit(1)
	}
}

func scanCmd() {
	var (
		timeout     = flag.Duration("timeout", 5*time.Second, "Query timeout per server")
		format      = flag.String("format", "text", "Output format (text, json)")
		players     = flag.Bool("players", false, "Include player list")
		portStart   = flag.Int("port-start", 0, "Start of port range to scan")
		portEnd     = flag.Int("port-end", 0, "End of port range to scan")
		ports       = flag.String("ports", "", "Comma-separated list of ports to scan")
		concurrency = flag.Int("concurrency", 10, "Maximum concurrent queries")
		noProgress  = flag.Bool("no-progress", false, "Disable progress indicator")
		debug       = flag.Bool("debug", false, "Enable debug logging")
	)
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		showHelp()
		return
	}

	address := args[0]
	ctx, cancel := context.WithTimeout(context.Background(), *timeout*10) // Allow more time for scanning
	defer cancel()

	// Build options
	var opts []query.Option
	opts = append(opts, query.WithTimeout(*timeout))
	opts = append(opts, query.WithMaxConcurrency(*concurrency))

	if *players {
		opts = append(opts, query.WithPlayers())
	}

	if *debug {
		opts = append(opts, query.WithDebug())
	}

	// Handle port options
	if *ports != "" {
		// Parse custom ports
		portList := []int{}
		for _, p := range strings.Split(*ports, ",") {
			var port int
			if _, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &port); err == nil {
				portList = append(portList, port)
			}
		}
		if len(portList) > 0 {
			opts = append(opts, query.WithPorts(portList))
		}
	} else if *portStart > 0 && *portEnd >= *portStart {
		// Use port range
		opts = append(opts, query.WithPortRange(*portStart, *portEnd))
	}
	// Otherwise, scan all default ports (default behavior)

	// Use progress indicator unless disabled or JSON format
	showProgress := !*noProgress && *format != "json"

	var servers []*protocol.ServerInfo
	var err error

	if showProgress {
		// Use progress-enabled version
		progressChan := make(chan query.ScanProgress, 100)

		// Start progress display in separate goroutine
		progressDone := make(chan struct{})
		go func() {
			defer close(progressDone)

			for progress := range progressChan {
				if progress.TotalPorts == 0 {
					// Discovery phase - show ports being checked
					fmt.Fprintf(os.Stderr, "\r\033[KDiscovering ports... Checked %d ports, found %d server(s)",
						progress.Completed, progress.ServersFound)
				} else {
					// Final scanning phase - show percentage
					totalScans := progress.TotalPorts * progress.TotalProtocols
					remaining := totalScans - progress.Completed
					percentage := 0
					if totalScans > 0 {
						percentage = (progress.Completed * 100) / totalScans
					}

					fmt.Fprintf(os.Stderr, "\r\033[K[%d%%] Scanning %d ports... Found %d server(s), %d scans remaining",
						percentage, progress.TotalPorts, progress.ServersFound, remaining)
				}

				// Force output to appear immediately
				os.Stderr.Sync()
			}

			// Final update - clear the progress line completely
			fmt.Fprintf(os.Stderr, "\r\033[K")
			// Move cursor to start of line and clear any remaining content
			fmt.Fprintf(os.Stderr, "\r")
		}()

		servers, err = query.DiscoverServersWithProgress(ctx, address, progressChan, opts...)
		<-progressDone // Wait for progress display to finish

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Print a newline after progress is done
		fmt.Fprintln(os.Stderr)
	} else {
		// Use regular version without progress
		servers, err = query.DiscoverServers(ctx, address, opts...)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	if len(servers) == 0 {
		fmt.Println("No game servers found")
		return
	}

	if err := outputScanResults(servers, *format); err != nil {
		fmt.Fprintf(os.Stderr, "Output error: %v\n", err)
		os.Exit(1)
	}
}

func showHelp() {
	fmt.Printf(`GameserverQuery - Query game servers for status information

Usage:
  gameserverquery [options] <address[:port]>    # Query a single server
  gameserverquery scan [options] <address>      # Scan for multiple servers
  gameserverquery list                          # List supported games

Common Options:
  -timeout duration    Query timeout (default 5s)
  -format string       Output format: text, json (default "text")
  -players             Include player list
  -debug               Enable debug logging

Query Options:
  -game string         Game type (auto-detect if not specified)

Scan Options:
  -port-start int      Start of port range to scan
  -port-end int        End of port range to scan
  -ports string        Comma-separated list of ports to scan
  -concurrency int     Maximum concurrent queries (default 10)
  -no-progress         Disable progress indicator

Examples:
  gameserverquery play.hypixel.net                        # Query gameserver (auto-detect)
  gameserverquery play.hypixel.net -players               # Include players list
  gameserverquery -game minecraft play.hypixel.net:25565  # Query gameserver with port and/or game, faster
  gameserverquery -game ark-survival-evolved server.com   # Uses query port 27015 automatically
  gameserverquery scan 127.0.0.1                          # Scan address for gameservers
`)
}

func listGames() {
	games := query.SupportedGames()
	sort.Strings(games)

	fmt.Println("Supported games:")
	for _, game := range games {
		gamePort := query.DefaultPort(game)
		queryPort := query.DefaultQueryPort(game)
		if gamePort == queryPort {
			fmt.Printf("  %-15s (port: %d)\n", game, gamePort)
		} else {
			fmt.Printf("  %-15s (game: %d, query: %d)\n", game, gamePort, queryPort)
		}
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
	fmt.Printf("Query Port: %d\n", info.QueryPort)
	fmt.Printf("Players: %d/%d\n", info.Players.Current, info.Players.Max)
	fmt.Printf("Ping: %d\n", info.Ping)

	// Optional fields
	printIfNotEmpty("Map", info.Map)
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

func outputScanResults(servers []*protocol.ServerInfo, format string) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(servers)
	case "text":
		return outputScanText(servers)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func outputScanText(servers []*protocol.ServerInfo) error {
	fmt.Printf("Found %d game server(s)\n\n", len(servers))

	for i, info := range servers {
		if i > 0 {
			fmt.Println(strings.Repeat("-", 50))
		}

		fmt.Printf("Server #%d\n", i+1)
		if info.Name != "" {
			fmt.Printf("  Name: %s\n", info.Name)
		}
		fmt.Printf("  Game: %s\n", info.Game)
		fmt.Printf("  Address: %s:%d\n", info.Address, info.Port)
		fmt.Printf("  Query Port: %d\n", info.QueryPort)
		fmt.Printf("  Players: %d/%d\n", info.Players.Current, info.Players.Max)
		if info.Version != "" {
			fmt.Printf("  Version: %s\n", info.Version)
		}
		if info.Map != "" {
			fmt.Printf("  Map: %s\n", info.Map)
		}
		if info.Ping > 0 {
			fmt.Printf("  Ping: %dms\n", info.Ping)
		}

		// Show player list if available
		if len(info.Players.List) > 0 {
			fmt.Printf("  Players:\n")
			for _, player := range info.Players.List {
				fmt.Printf("    - %s", player.Name)
				if player.Score > 0 {
					fmt.Printf(" (Score: %d)", player.Score)
				}
				if player.Duration > 0 {
					fmt.Printf(" (Time: %v)", player.Duration)
				}
				fmt.Println()
			}
		}
	}

	return nil
}

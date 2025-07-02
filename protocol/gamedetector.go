package protocol

import (
	"fmt"
	"strings"
)

// GameDetector handles centralized game detection from server responses
type GameDetector struct{}

// DetectGame analyzes server response data to determine the actual game
func (gd *GameDetector) DetectGame(info *ServerInfo, protocolName string) string {
	if info == nil || !info.Online {
		return protocolName
	}

	// Protocol-specific detection logic
	switch protocolName {
	case "minecraft":
		return gd.detectMinecraft(info)
	case "terraria":
		return gd.detectTerraria(info)
	case "source", "rust":
		return gd.detectSourceGame(info)
	default:
		return protocolName
	}
}

// detectMinecraft handles Minecraft game detection
func (gd *GameDetector) detectMinecraft(info *ServerInfo) string {
	// Minecraft protocol is pretty specific, if it responds it's likely Minecraft
	// Could add version-based detection here (Forge, Bukkit, etc.)
	return "minecraft"
}

// detectTerraria handles Terraria game detection
func (gd *GameDetector) detectTerraria(info *ServerInfo) string {
	// Terraria protocol is specific to Terraria
	return "terraria"
}

// detectSourceGame handles Source engine game detection
func (gd *GameDetector) detectSourceGame(info *ServerInfo) string {
	// Extract game description and App ID from Extra data if available
	gameDesc := ""
	appIDStr := ""
	if info.Extra != nil {
		if desc, exists := info.Extra["game"]; exists {
			gameDesc = desc
		}
		if id, exists := info.Extra["app_id"]; exists {
			appIDStr = id
		}
	}
	
	// Try App ID detection first (most reliable)
	if appIDStr != "" {
		if game := gd.detectByAppID(appIDStr); game != "" {
			return game
		}
	}
	
	// Fallback to game description analysis
	if gameDesc == "" {
		// If no game description, try to extract from server name
		gameDesc = info.Name
	}
	
	return gd.analyzeGameDescription(gameDesc)
}

// detectByAppID determines game type from Steam App ID
func (gd *GameDetector) detectByAppID(appIDStr string) string {
	// Convert string to int for comparison
	var appID int
	if _, err := fmt.Sscanf(appIDStr, "%d", &appID); err != nil {
		return ""
	}
	
	// Check by App ID first (most reliable)
	// Note: Some newer games have App IDs that exceed uint16 range
	switch appID {
	case 730:
		return "counter-strike"
	case 240:
		return "counter-source"
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
		return "day-of-defeat-source"
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

// analyzeGameDescription determines game type from description/name
func (gd *GameDetector) analyzeGameDescription(gameDesc string) string {
	if gameDesc == "" {
		return "source"
	}
	
	gameLower := strings.ToLower(gameDesc)
	
	// Check for specific games in order of specificity
	if strings.Contains(gameLower, "counter-strike 2") || strings.Contains(gameLower, "cs2") {
		return "counter-strike-2"
	} else if strings.Contains(gameLower, "counter-strike: global offensive") || strings.Contains(gameLower, "csgo") {
		return "counter-strike"
	} else if strings.Contains(gameLower, "counter-strike") || strings.Contains(gameLower, "cs:") {
		return "counter-strike"
	} else if strings.Contains(gameLower, "garrysmod") || strings.Contains(gameLower, "garry") || strings.Contains(gameLower, "gmod") {
		return "garrys-mod"
	} else if strings.Contains(gameLower, "team fortress") || strings.Contains(gameLower, "tf2") {
		return "team-fortress-2"
	} else if strings.Contains(gameLower, "left 4 dead 2") || strings.Contains(gameLower, "l4d2") {
		return "left-4-dead-2"
	} else if strings.Contains(gameLower, "left 4 dead") || strings.Contains(gameLower, "l4d") {
		return "left-4-dead"
	} else if strings.Contains(gameLower, "rust") {
		return "rust"
	} else if strings.Contains(gameLower, "ark") || strings.Contains(gameLower, "survival evolved") {
		return "ark-survival-evolved"
	} else if strings.Contains(gameLower, "insurgency") {
		return "insurgency"
	} else if strings.Contains(gameLower, "day of defeat") || strings.Contains(gameLower, "dod") {
		return "day-of-defeat"
	} else if strings.Contains(gameLower, "project zomboid") || strings.Contains(gameLower, "zomboid") {
		return "project-zomboid"
	} else if strings.Contains(gameLower, "satisfactory") {
		return "satisfactory"
	} else if strings.Contains(gameLower, "7 days to die") || strings.Contains(gameLower, "7dtd") {
		return "7-days-to-die"
	} else if strings.Contains(gameLower, "valheim") {
		return "valheim"
	} else if strings.Contains(gameLower, "arma 3") || strings.Contains(gameLower, "arma3") {
		return "arma-3"
	} else if strings.Contains(gameLower, "dayz") || strings.Contains(gameLower, "day z") {
		return "dayz"
	} else if strings.Contains(gameLower, "battalion 1944") || strings.Contains(gameLower, "battalion1944") {
		return "battalion-1944"
	} else if strings.Contains(gameLower, "half-life") || strings.Contains(gameLower, "hl2") {
		return "half-life"
	}
	
	// Default to generic source
	return "source"
}

// Global detector instance
var detector = &GameDetector{}

// DetectGameFromResponse is the main entry point for game detection
func DetectGameFromResponse(info *ServerInfo, protocolName string) string {
	return detector.DetectGame(info, protocolName)
}
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var config = configT{
	AppTitle: "RIGOL Scope Capture",
	AppName:  "scope_capture",
	Ip:       "169.254.247.73",
	Port:     5555,
	// Hostname is assigned at runtime
	Hostname: "",
}

type configT struct {
	AppTitle string
	AppName  string
	Port     int
	Ip       string
	Hostname string
}

// fileConfig is used only for unmarshaling JSON
type fileConfig struct {
	Ip   string `json:"ip"`
	Port int    `json:"port"`
}

// loadAndParseConfigFile tries to load configuration from either
// ./scope_config.json or ~/.config/scope_config/scope_config.json
func loadAndParseConfigFile() error {
	// Build the two possible paths to the config file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// If we can't determine the home directory, just skip the second path
		homeDir = ""
	}
	xdgConfigPath := filepath.Join(homeDir, ".config", "scope_config", "scope_config.json")

	pathsToTry := []string{
		"./scope_config.json",
		xdgConfigPath,
	}

	// Try each path in turn
	for _, path := range pathsToTry {
		if _, err := os.Stat(path); err == nil {
			log.InfoPrintf("Found config file at %q. Loading it...", path)
			// The file exists, attempt to load & parse it
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("Unable to load config file: %v", err)
			}
			defer file.Close()

			var fc fileConfig
			if err := json.NewDecoder(file).Decode(&fc); err != nil {
				return fmt.Errorf("Unable to parse config JSON: %v", err)
			}

			// Overwrite fields only if they're present (non-empty / non-zero)
			if fc.Ip != "" {
				config.Ip = fc.Ip
				log.InfoPrintf("    Using IP from config file: %q", fc.Ip)
			}
			if fc.Port != 0 {
				config.Port = fc.Port
				log.InfoPrintf("    Using port from config file: %d", fc.Port)
			}

			// Once we've successfully read one config file, we stop
			return nil
		}
	}
	// If we got here, no config file was found in either path.
	log.InfoPrint("No config file found. Using default values.")
	return nil
}

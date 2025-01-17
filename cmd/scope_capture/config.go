package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var config = configT{
	AppTitle:      "RIGOL Scope Capture",
	AppName:       "scope_capture",
	ScopeHostname: "169.254.247.73",
	ScopePort:     5555,
	// Hostname is assigned at runtime
	Hostname: "",
}

type configT struct {
	AppTitle      string
	AppName       string
	ScopePort     int
	ScopeHostname string
	Hostname      string
}

// fileConfig is used only for unmarshaling JSON
type fileConfig struct {
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
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
	xdgConfigPath := filepath.Join(homeDir, ".config", "scope_capture", "scope_config.json")

	pathsToTry := []string{
		"./scope_config.json",
		xdgConfigPath,
	}

	// Try each path in turn
	for _, path := range pathsToTry {
		log.InfoPrintf("Checking for config file at %q...", path)
		if _, err := os.Stat(path); err == nil {
			log.InfoPrint("    Found. Loading...")
			// The file exists, attempt to load & parse it
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("unable to load config file: %v", err)
			}
			defer file.Close()

			var fc fileConfig
			if err := json.NewDecoder(file).Decode(&fc); err != nil {
				return fmt.Errorf("unable to parse config JSON: %v", err)
			}

			// Overwrite fields only if they're present (non-empty / non-zero)
			itemsFound := false
			if fc.Hostname != "" {
				config.ScopeHostname = fc.Hostname
				log.InfoPrintf("        Adopting scope hostname from config file: %q", fc.Hostname)
				itemsFound = true
			}
			if fc.Port != 0 {
				config.ScopePort = fc.Port
				log.InfoPrintf("        Adopting scope port from config file: %d", fc.Port)
				itemsFound = true
			}
			if !itemsFound {
				log.InfoPrint("        WARNING: No (known) configuration items found in config file.")
			}

			// Once we've successfully read one config file, we stop
			log.Debugf("config: %#v", config)
			return nil
		}
	}
	// If we got here, no config file was found in either path.
	log.InfoPrint("No config file found. Using default values.")
	return nil
}

package modeline

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Config structure for storing settings from modeline
type Config struct {
	Nodes     []string
	Endpoints []string
	Templates []string
}

// ParseModeline parses a modeline string and populates the Config structure
func ParseModeline(line string) (*Config, error) {
	config := &Config{}
	trimLine := strings.TrimSpace(line)
	prefix := "# talm: "
	if strings.HasPrefix(trimLine, prefix) {
		content := strings.TrimPrefix(trimLine, prefix)
		parts := strings.Split(content, ", ")
		for _, part := range parts {
			keyVal := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(keyVal) != 2 {
				return nil, fmt.Errorf("invalid format of modeline part: %s", part)
			}
			key := keyVal[0]
			val := keyVal[1]
			var arr []string
			if err := json.Unmarshal([]byte(val), &arr); err != nil {
				return nil, fmt.Errorf("error parsing JSON array for key %s, value %s, error: %v", key, val, err)
			}
			// Assign values to Config fields based on known keys
			switch key {
			case "nodes":
				config.Nodes = arr
			case "endpoints":
				config.Endpoints = arr
			case "templates":
				config.Templates = arr
				// Ignore unknown keys
			}
		}
		return config, nil
	}
	return nil, fmt.Errorf("modeline prefix not found")
}

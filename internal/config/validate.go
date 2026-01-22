package config

import (
	"fmt"
	"reflect"
	"strings"
)

// ValidateKeys checks for duplicate keybindings and invalid key strings.
func ValidateKeys(keys *KeyBindings) error {
	// Build a map of key -> action names for duplicate detection
	keyMap := make(map[string][]string)

	v := reflect.ValueOf(keys).Elem()
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldName := t.Field(i).Name

		if field.Kind() != reflect.String {
			continue
		}

		keyStr := field.String()
		if keyStr == "" {
			continue
		}

		// Validate that the key string can be parsed
		_, err := ParseKeyPreserveCase(keyStr)
		if err != nil {
			return fmt.Errorf("invalid key for %s: %w", fieldName, err)
		}

		// Normalize the key for duplicate detection
		// Lowercase for regular chars, but preserve uppercase for shift keys
		normalizedKey := keyStr
		keyMap[normalizedKey] = append(keyMap[normalizedKey], fieldName)
	}

	// Check for duplicates
	var duplicates []string
	for key, actions := range keyMap {
		if len(actions) > 1 {
			duplicates = append(duplicates, fmt.Sprintf("key %q is used by: %s", key, strings.Join(actions, ", ")))
		}
	}

	if len(duplicates) > 0 {
		return fmt.Errorf("duplicate keybindings found:\n  %s", strings.Join(duplicates, "\n  "))
	}

	return nil
}

// ValidateColor checks if a color string is valid for gocui.
func ValidateColor(color string) bool {
	validColors := map[string]bool{
		"default": true,
		"black":   true,
		"red":     true,
		"green":   true,
		"yellow":  true,
		"blue":    true,
		"magenta": true,
		"cyan":    true,
		"white":   true,
	}
	return validColors[strings.ToLower(color)]
}

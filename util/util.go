package util

import (
	"regexp"
	"strings"
)

// NormalizeDeviceID takes a platform-specific BLE address (UUID on macOS,
// MAC on Windows/Linux) and normalizes it into a consistent format.
func NormalizeDeviceID(raw string) string {
	raw = strings.TrimSpace(raw)

	// Case 1: MAC address (Windows/Linux)
	macLike := regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`)
	if macLike.MatchString(raw) {
		// Normalize to uppercase with colons
		return strings.ToUpper(strings.ReplaceAll(raw, "-", ":"))
	}

	// Case 2: UUID (macOS)
	uuidLike := regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)
	if uuidLike.MatchString(raw) {
		// Take the first 12 hex chars, format like MAC
		clean := strings.ReplaceAll(raw, "-", "")
		if len(clean) >= 12 {
			clean = clean[:12]
			parts := []string{}
			for i := 0; i < 12; i += 2 {
				parts = append(parts, clean[i:i+2])
			}
			return strings.ToUpper(strings.Join(parts, ":"))
		}
	}

	// Fallback: just return uppercased raw
	return strings.ToUpper(raw)
}

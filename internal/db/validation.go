package db

import "fmt"

// ValidateStatusAndError verifies standard status/error invariants across judgment models (FR-007, FR-105).
// In status="error", error string must be non-empty.
// In status="ok", error string must be empty.
func ValidateStatusAndError(status, errStr, itemDesc string) error {
	switch status {
	case "ok", "error":
	default:
		return fmt.Errorf("%s: bad status %q", itemDesc, status)
	}

	if status == "error" {
		if errStr == "" {
			return fmt.Errorf("%s: status=error requires non-empty error", itemDesc)
		}
		return nil
	}

	if errStr != "" {
		return fmt.Errorf("%s: status=ok must not carry error", itemDesc)
	}
	return nil
}

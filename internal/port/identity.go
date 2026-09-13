package port

import (
	"strings"
	"unicode/utf8"
)

// Byte limits match VARBINARY identity columns. Values are never trimmed or
// case-folded: callers may distinguish "Key", "key" and "key ".
const (
	MaxRunIDBytes           = 64
	MaxStrategyIDBytes      = 64
	MaxStrategyVersionBytes = 32
	MaxEngineVersionBytes   = 32
	MaxHashBytes            = 64
	MaxIdempotencyKeyBytes  = 128
	MaxLeaseIdentityBytes   = 128
	MaxSnapshotIDBytes      = 64
	MaxEventIDBytes         = 64
	MaxAggregateIDBytes     = 128
	MaxMarketSourceBytes    = 128
	MaxSourceEventIDBytes   = 128
)

// ValidateIdentity also serves adapters whose methods accept scalar keys rather
// than a complete Run/Snapshot value. Optional empty lease fields are allowed.
func ValidateIdentity(value, field string, maxBytes int, allowEmpty bool) error {
	if allowEmpty && value == "" {
		return nil
	}
	if strings.TrimSpace(value) == "" || !utf8.ValidString(value) || len(value) > maxBytes {
		return invalidPortValue("%s must be nonblank valid UTF-8 within %d bytes", field, maxBytes)
	}
	return nil
}

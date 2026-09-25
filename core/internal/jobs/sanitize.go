package jobs

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/zajuna-app/core/internal/security"
)

// maxPersistedMessageRunes bounds error/progress text stored in SQLite and
// shown in the UI; worker errors can wrap whole HTTP bodies.
const maxPersistedMessageRunes = 2000

// SanitizeMessage prepares worker-provided text for persistence: secrets in
// key=value, cookie or bearer form are redacted, control characters are
// dropped and the result is bounded.
func SanitizeMessage(message string) string {
	message = security.RedactText(message)
	message = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, message)
	message = strings.TrimSpace(message)
	if utf8.RuneCountInString(message) > maxPersistedMessageRunes {
		runes := []rune(message)
		message = string(runes[:maxPersistedMessageRunes]) + "…"
	}
	return message
}

// encodeOutput marshals a worker output, redacting secrets from every string
// value so a result never persists what an error message could not.
func encodeOutput(value any) (json.RawMessage, error) {
	contents, err := json.Marshal(value)
	if err != nil || value == nil {
		return contents, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	var generic any
	if err := decoder.Decode(&generic); err != nil {
		return contents, nil
	}
	return json.Marshal(redactStrings(generic))
}

func redactStrings(value any) any {
	switch typed := value.(type) {
	case string:
		return security.RedactText(typed)
	case []any:
		for i, item := range typed {
			typed[i] = redactStrings(item)
		}
		return typed
	case map[string]any:
		for key, item := range typed {
			typed[key] = redactStrings(item)
		}
		return typed
	default:
		return value
	}
}

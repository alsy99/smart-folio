package research

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode"

	"aperture/pkg/llm"
)

func HeadlineHash(headline string) string {
	norm := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return unicode.ToLower(r)
	}, strings.TrimSpace(headline))
	norm = strings.Join(strings.Fields(norm), " ")
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:8])
}

func CacheKey(symbol, day, headline string) string {
	if symbol == "" {
		symbol = "_"
	}
	if day == "" {
		day = "undated"
	}
	return symbol + "|" + day + "|" + HeadlineHash(headline)
}

func IDForKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "inv-" + hex.EncodeToString(sum[:6])
}

func Day(now time.Time) string {
	return llm.ISTDay(now)
}

var orderFields = []string{
	"qty", "quantity", "order", "orders", "side", "notional",
	"limit_price", "stop_price", "stop", "broker", "action", "lots",
}

// StripOrderJSON drops broker-intent keys so an LLM cannot emit an order.
func StripOrderJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw
	}
	for _, k := range orderFields {
		delete(m, k)
		delete(m, strings.ToUpper(k))
	}
	b, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return string(b)
}

package scraper

import "strings"

// ParseCookieHeader parses a "name=value; name2=value2" cookie string into a map.
// Whitespace around names/values is trimmed; empty/malformed pairs are skipped.
// Values containing "=" are handled correctly by splitting on the first "=" only.
func ParseCookieHeader(s string) map[string]string {
	result := map[string]string{}
	for _, pair := range strings.Split(s, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		idx := strings.Index(pair, "=")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(pair[:idx])
		value := strings.TrimSpace(pair[idx+1:])
		if name == "" {
			continue
		}
		result[name] = value
	}
	return result
}

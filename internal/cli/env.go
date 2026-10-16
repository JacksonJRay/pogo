package cli

import (
	"strings"

	"pogo/internal/dotenv"
)

// buildEnv merges decrypted pairs into the current environment.
//
// Existing variables use POSIX first-match rules. When override is true, pairs
// replace those values. When override is false, an existing value is kept.
// Keys in drop are removed from the existing environment first, so a passphrase
// variable is not passed to the child. If the encrypted file also contains that
// key, the file value is still applied.
func buildEnv(base []string, pairs []dotenv.Pair, drop map[string]struct{}, override bool) []string {
	order := make([]string, 0, len(base)+len(pairs))
	idx := make(map[string]int, len(base)+len(pairs))
	for _, e := range base {
		k, v, ok := strings.Cut(e, "=")
		if !ok || k == "" {
			continue
		}
		if _, skip := drop[k]; skip {
			continue
		}
		if _, exists := idx[k]; exists {
			continue
		}
		idx[k] = len(order)
		order = append(order, k+"="+v)
	}
	for _, p := range pairs {
		kv := p.Key + "=" + p.Value
		if i, exists := idx[p.Key]; exists {
			if override {
				order[i] = kv
			}
			continue
		}
		idx[p.Key] = len(order)
		order = append(order, kv)
	}
	return order
}

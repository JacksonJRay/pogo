package cli

import "strings"

func suggest(input string, options []string) string {
	if input == "" || len(options) == 0 {
		return ""
	}
	best := ""
	bestD := 1 << 30
	in := strings.ToLower(input)
	for _, opt := range options {
		d := levenshtein(in, strings.ToLower(opt))
		if d < bestD {
			bestD = d
			best = opt
		}
	}
	limit := 1
	if len([]rune(input)) >= 5 {
		limit = 2
	}
	if best != "" && bestD <= limit && bestD > 0 {
		return `"` + best + `"`
	}
	return ""
}

func levenshtein(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := cur[j-1] + 1
			sub := prev[j-1] + cost
			cur[j] = min(del, ins, sub)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

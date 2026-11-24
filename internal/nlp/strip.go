package nlp

import (
	"regexp"
	"strings"
	"sync"
)

// stripPhrases removes each phrase from text, matching whole words only.
//
// This replaces strings.Replace, which matches anywhere and quietly ate the
// middle of ordinary words. Removing "me" turned "meant" into "ant", so
// "I meant to just greet you" was parsed as a search for "i ant to just greet
// you". Removing "the" turned "theory" into "ory", which means every question
// about theory notes had been searching for a word that does not exist. The
// damage was invisible because the result is still a plausible looking string.
//
// Phrases are tried longest first, so "can you find me" is consumed before
// "find" gets a chance to break it into pieces.
func stripPhrases(text string, phrases []string) string {
	out := text
	for _, re := range strippers(phrases) {
		out = re.ReplaceAllString(out, " ")
	}
	return strings.Join(strings.Fields(out), " ")
}

var (
	stripCache   = map[string][]*regexp.Regexp{}
	stripCacheMu sync.Mutex
)

// strippers compiles one boundary-anchored pattern per phrase, longest first,
// and keeps them: this runs on every query and recompiling a dozen patterns
// each time is pure waste.
func strippers(phrases []string) []*regexp.Regexp {
	key := strings.Join(phrases, "\x00")
	stripCacheMu.Lock()
	defer stripCacheMu.Unlock()
	if got, ok := stripCache[key]; ok {
		return got
	}

	sorted := append([]string(nil), phrases...)
	// Longest first. Otherwise "find" fires inside "can you find me" and
	// leaves "can you me", which the shorter patterns then mangle further.
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && len(sorted[j]) > len(sorted[j-1]); j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	out := make([]*regexp.Regexp, 0, len(sorted))
	for _, p := range sorted {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(p) + `\b`)
		if err != nil {
			continue
		}
		out = append(out, re)
	}
	stripCache[key] = out
	return out
}

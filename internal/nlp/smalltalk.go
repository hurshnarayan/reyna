package nlp

import (
	"strings"
	"unicode"
)

// pleasantries are words that carry no request.
var pleasantries = map[string]bool{
	"hi": true, "hii": true, "hiii": true, "hello": true, "helo": true,
	"hey": true, "heyy": true, "yo": true, "hola": true, "namaste": true,
	"greetings": true, "sup": true,
	"thanks": true, "thank": true, "thanku": true, "thankyou": true,
	"ty": true, "thx": true, "cheers": true,
	"ok": true, "okay": true, "k": true, "kk": true, "cool": true,
	"nice": true, "great": true, "good": true, "fine": true, "perfect": true,
	"morning": true, "afternoon": true, "evening": true, "night": true,
	"bye": true, "goodbye": true, "cya": true,
	"you": true, "u": true, "there": true, "a": true, "lot": true, "so": true, "much": true,
	"test": true, "testing": true,
	// Indic romanized & native pleasantries
	"dhanyawad": true, "dhanyawaad": true, "dhanyavad": true, "shukriya": true, "shukriyaa": true,
	"pranam": true, "pranaam": true, "vanakkam": true, "namaskaram": true, "alvida": true,
	"नमस्ते": true, "धन्यवाद": true, "शुक्रिया": true, "प्रणाम": true, "வணக்கம்": true, "నమస్కారం": true,
}

// IsSmallTalk reports whether a message is a greeting or an acknowledgement
// rather than a request for a document.
//
// Without this, "thanks" was parsed as a topic and searched for, and six
// unrelated documents came back with "I found 6 documents matching thanks, and
// they are about different things. Which one did you mean?". Saying thank you
// should not produce a disambiguation prompt.
//
// Deliberately narrow, and it errs towards saying no. A greeting misread as a
// question only wastes a search; a question misread as a greeting refuses to
// look for a document somebody actually wants. So every word has to be a
// pleasantry and there can be at most four of them: "thanks a lot" and "good
// morning" qualify, "good notes on databases" does not.
func IsSmallTalk(text string) bool {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r)
	})
	if len(words) == 0 || len(words) > 4 {
		return false
	}
	for _, w := range words {
		if !pleasantries[w] {
			return false
		}
	}
	return true
}

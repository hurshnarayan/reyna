package nlp

import "testing"

func TestIsSmallTalk(t *testing.T) {
	yes := []string{"hi", "Hello!", "hey there", "thanks", "thanks a lot", "ok", "good morning", "bye", "dhanyawad", "shukriya", "namaste", "नमस्ते", "धन्यवाद"}
	for _, s := range yes {
		if !IsSmallTalk(s) {
			t.Errorf("expected small talk: %q", s)
		}
	}
	// The direction that matters. A question read as a greeting refuses to
	// look for a document the user actually wants.
	no := []string{
		"any notes on dbms",
		"good notes on databases",
		"hey can you find my ode notes",
		"what is module 1 about",
		"thanks for the module 4 notes, where is module 5",
		"test paper for AI",
		"",
	}
	for _, s := range no {
		if IsSmallTalk(s) {
			t.Errorf("expected NOT small talk: %q", s)
		}
	}
}

// strings.Replace matched anywhere, so removing "me" turned "meant" into
// "ant" and removing "the" turned "theory" into "ory".
func TestStripPhrasesKeepsWholeWordsIntact(t *testing.T) {
	cases := []struct{ in, want string }{
		{"i meant to just greet you", "i meant to just greet you"},
		{"module 1 theory notes", "module 1 theory notes"},
		{"company handbook", "company handbook"},
		{"can you find me the dbms notes", "dbms notes"},
		{"show me any recent theory paper", "theory paper"},
	}
	phrases := []string{
		"can you find me", "can you show me", "can you get me", "can you find", "can you",
		"find me", "show me", "get me", "search for", "find", "search",
		"has anyone shared", "do we have", "share", "shared", "upload", "uploaded",
		"sent", "send", "received", "receive", "about", "any", "the", "some",
		"latest", "recently", "recent", "me",
	}
	for _, c := range cases {
		if got := stripPhrases(c.in, phrases); got != c.want {
			t.Errorf("stripPhrases(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

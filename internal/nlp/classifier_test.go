package nlp

import (
	"testing"
)

func TestKeywordParseQueryReynaScript(t *testing.T) {
	c := New(nil)
	testCases := []struct {
		query    string
		wantWho  string
		wantWhat string
		wantWhen string
		wantWhy  string
	}{
		{
			query:    "can you find me the latest Reyna script received",
			wantWho:  "",
			wantWhat: "reyna script",
			wantWhen: "",
			wantWhy:  "search",
		},
		{
			query:    "find me the latest Reyna script",
			wantWho:  "",
			wantWhat: "reyna script",
			wantWhen: "",
			wantWhy:  "search",
		},
		{
			query:    "find the reyna script",
			wantWho:  "",
			wantWhat: "reyna script",
			wantWhen: "",
			wantWhy:  "search",
		},
		{
			query:    "latest reyna script received",
			wantWho:  "",
			wantWhat: "reyna script",
			wantWhen: "",
			wantWhy:  "retrieve",
		},
		{
			query:    "reyna script",
			wantWho:  "",
			wantWhat: "reyna script",
			wantWhen: "",
			wantWhy:  "retrieve",
		},
		{
			query:    "mohit sent some notes",
			wantWho:  "mohit",
			wantWhat: "",
			wantWhen: "",
			wantWhy:  "retrieve",
		},
		{
			query:    "what did priya share yesterday",
			wantWho:  "priya",
			wantWhat: "",
			wantWhen: "yesterday",
			wantWhy:  "retrieve",
		},
		{
			query:    "can you get me train ticket to hyderabad",
			wantWho:  "",
			wantWhat: "train ticket to hyderabad",
			wantWhen: "",
			wantWhy:  "fetch",
		},
		{
			query:    "please send me module 4",
			wantWho:  "",
			wantWhat: "module 4",
			wantWhen: "",
			wantWhy:  "fetch",
		},
	}

	for _, tc := range testCases {
		who, what, when, why := c.ParseNLPQuery(tc.query)
		if who != tc.wantWho {
			t.Errorf("Query %q: who = %q, want %q", tc.query, who, tc.wantWho)
		}
		if what != tc.wantWhat {
			t.Errorf("Query %q: what = %q, want %q", tc.query, what, tc.wantWhat)
		}
		if when != tc.wantWhen {
			t.Errorf("Query %q: when = %q, want %q", tc.query, when, tc.wantWhen)
		}
		if why != tc.wantWhy {
			t.Errorf("Query %q: why = %q, want %q", tc.query, why, tc.wantWhy)
		}
	}
}

func TestNormalizeRetrievalIntentKeepsQuestionsAsQuestions(t *testing.T) {
	if got := normalizeRetrievalIntent("what does the train ticket say?", "qa"); got != "qa" {
		t.Fatalf("intent = %q, want qa", got)
	}
}

// A transcription cut off at the token ceiling still holds a complete reading
// of everything before the cut. Discarding it as a parse error is what made a
// perfectly ordinary PDF look like a document containing nothing.
func TestSalvageJSONStringRecoversTruncatedContent(t *testing.T) {
	truncated := `{"content": "[[page 1]]\nFirst order ODE\nVariable separable form`
	got := salvageJSONString(truncated, "content")
	want := "[[page 1]]\nFirst order ODE\nVariable separable form"
	if got != want {
		t.Fatalf("salvage = %q, want %q", got, want)
	}
}

func TestSalvageJSONStringHonoursEscapes(t *testing.T) {
	got := salvageJSONString(`{"content": "he said \"yes\" and a path C:\\tmp`, "content")
	want := `he said "yes" and a path C:\tmp`
	if got != want {
		t.Fatalf("salvage = %q, want %q", got, want)
	}
}

func TestSalvageJSONStringStopsAtProperClose(t *testing.T) {
	got := salvageJSONString(`{"content": "all of it", "summary": "x"}`, "content")
	if got != "all of it" {
		t.Fatalf("salvage = %q, want %q", got, "all of it")
	}
}

func TestSalvageJSONStringMissingField(t *testing.T) {
	if got := salvageJSONString(`{"summary": "x"}`, "content"); got != "" {
		t.Fatalf("salvage = %q, want empty", got)
	}
}

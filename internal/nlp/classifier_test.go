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

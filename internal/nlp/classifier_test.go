package nlp

import (
	"testing"
)

type fixedDocProvider struct {
	response string
	docCalls *int
}

func (p fixedDocProvider) Complete(string, int) (string, error) {
	return "", nil
}

func (p fixedDocProvider) CompleteWithDoc(string, []byte, string, int) (string, error) {
	if p.docCalls != nil {
		*p.docCalls++
	}
	return p.response, nil
}

func (p fixedDocProvider) Embed(string) ([]float32, error) {
	return nil, nil
}

func (p fixedDocProvider) Name() string {
	return "test"
}

func (p fixedDocProvider) IsEnabled() bool {
	return true
}

func TestPDFClassificationTrustsStrongContentOverFilenameAndModel(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		content  string
		model    string
		want     string
	}{
		{
			name:     "queue implementation is data structures",
			fileName: "prog5 circular queue.pdf",
			content:  "Study material about circular queue data structure implementation.",
			model:    "Probability And Statistics",
			want:     "Data Structures",
		},
		{
			name:     "probability content beats folder words in filename",
			fileName: "Data Structures probability.pdf",
			content:  "Discrete random variable and probability distribution examples.",
			model:    "Data Structures",
			want:     "Probability And Statistics",
		},
	}

	folders := []string{"Probability And Statistics", "Data Structures"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := `{"content":` + `"` + tt.content + `"` +
				`,"summary":"` + tt.content + `","folder":"` + tt.model +
				`","is_new":false,"confidence":0.95}`
			classifier := New(fixedDocProvider{response: response})
			folder, _, _, extracted, _ := classifier.ClassifyFileWithContent(
				tt.fileName, "application/pdf", []byte("%PDF-test"), folders, FileMeta{},
			)
			if folder != tt.want {
				t.Fatalf("folder = %q, want %q", folder, tt.want)
			}
			if extracted != tt.content {
				t.Fatalf("content = %q, want %q", extracted, tt.content)
			}
		})
	}
}

func TestPDFClassificationSendsFilesAbovePreviousInlineLimit(t *testing.T) {
	content := "Discrete random variable and probability distribution examples."
	response := `{"content":"` + content + `","summary":"` + content +
		`","folder":"Probability And Statistics","is_new":false,"confidence":0.95}`
	docCalls := 0
	classifier := New(fixedDocProvider{response: response, docCalls: &docCalls})
	data := make([]byte, 15*1024*1024)

	folder, _, _, extracted, _ := classifier.ClassifyFileWithContent(
		"probability notes.pdf", "application/pdf", data,
		[]string{"Probability And Statistics", "Data Structures"}, FileMeta{},
	)
	if folder != "Probability And Statistics" {
		t.Fatalf("folder = %q, want Probability And Statistics", folder)
	}
	if extracted != content {
		t.Fatalf("PDF content was not returned: %q", extracted)
	}
	if docCalls != 1 {
		t.Fatalf("CompleteWithDoc called %d times, want 1 for a file above the old 14 MiB cap", docCalls)
	}
}

func TestSubjectFromContentRequiresSpecificEvidence(t *testing.T) {
	classifier := New(nil)
	folders := []string{"Probability And Statistics", "Data Structures"}

	folder, isNew, ok := classifier.SubjectFromContent(
		"Study material on circular queue implementation.",
		folders,
	)
	if !ok || isNew || folder != "Data Structures" {
		t.Fatalf("SubjectFromContent = (%q, %v, %v), want (Data Structures, false, true)", folder, isNew, ok)
	}

	if _, _, ok := classifier.SubjectFromContent("Queue", folders); ok {
		t.Fatal("a generic mention of queue should not be enough to override classification")
	}
}

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

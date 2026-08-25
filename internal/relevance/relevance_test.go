package relevance

import (
	"strings"
	"testing"
)

func TestWordsSplitsLetterDigitBoundary(t *testing.T) {
	got := Words("Module4_part1_Arrays.pptx")
	want := []string{"module", "4", "part", "1", "arrays", "pptx"}
	if len(got) != len(want) {
		t.Fatalf("Words = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Words = %v, want %v", got, want)
		}
	}
}

// The exact failure from the screenshots: a question about ordinary
// differential equations returned a semiconductor lecture, because "ode" is a
// substring of "diodes".
func TestOdeDoesNotMatchDiodes(t *testing.T) {
	if MatchText("MODULE 1-SEMICONDUCTOR DIODES.pptx", "ode") {
		t.Fatal("ode matched diodes")
	}
	if !MatchText("2CSE Module 1 ODE of first order.pdf", "ode") {
		t.Fatal("ode did not match a file with ODE in the name")
	}
}

func TestShortTokenNeedsWholeWord(t *testing.T) {
	if MatchText("Module4_part1_Arrays.pptx", "ai") {
		t.Fatal("ai matched inside a longer word")
	}
	if !MatchText("AI Module 1.pdf", "ai") {
		t.Fatal("ai did not match itself")
	}
}

func TestPrefixMatchesPluralButNotCoincidence(t *testing.T) {
	if !MatchText("first order differentials.pdf", "differential") {
		t.Fatal("differential did not reach differentials")
	}
	if MatchText("diodes.pdf", "die") {
		t.Fatal("short token matched by prefix")
	}
}

// "module 1" must beat "Module4_part1", which contains both words but not
// next to each other.
func TestAdjacencySeparatesModule1FromPart1(t *testing.T) {
	tokens := []string{"module", "1"}
	real := Scored("Module 1-PPT-1.pptx", "", "", tokens)
	decoy := Scored("Module4_part1_Arrays.pptx", "", "", tokens)

	if real.Coverage != 1.0 || decoy.Coverage != 1.0 {
		t.Fatalf("expected both to cover the query: real=%v decoy=%v", real.Coverage, decoy.Coverage)
	}
	if real.Adjacent == 0 {
		t.Fatal("Module 1 was not recognised as adjacent")
	}
	if decoy.Adjacent != 0 {
		t.Fatal("Module4_part1 counted as adjacent")
	}
	if real.Score <= decoy.Score {
		t.Fatalf("decoy outranked the real match: real=%v decoy=%v", real.Score, decoy.Score)
	}
}

// The full screenshot case. Only one file covers all three tokens, so the
// floor must exclude the other two entirely rather than cite them.
func TestModule1OdeExcludesTheDecoys(t *testing.T) {
	tokens := []string{"module", "1", "ode"}
	cases := []struct {
		name  string
		cover float64
	}{
		{"2CSE Module 1 ODE of first order.pdf", 1.0},
		{"MODULE 1-SEMICONDUCTOR DIODES.pptx", 2.0 / 3.0},
		{"Module4_part1_Arrays.pptx", 2.0 / 3.0},
		{"2CSE Module 4 NS of ODE.docx", 2.0 / 3.0},
	}
	best := 0.0
	for _, c := range cases {
		got := Scored(c.name, "", "", tokens)
		if got.Coverage != c.cover {
			t.Fatalf("%s coverage = %v, want %v", c.name, got.Coverage, c.cover)
		}
		if got.Coverage > best {
			best = got.Coverage
		}
	}
	floor := Floor(best)
	for _, c := range cases {
		got := Scored(c.name, "", "", tokens)
		kept := got.Coverage >= floor
		want := c.name == "2CSE Module 1 ODE of first order.pdf"
		if kept != want {
			t.Fatalf("%s kept=%v, want %v (floor %v)", c.name, kept, want, floor)
		}
	}
}

// When nothing covers the whole query the best available must survive, or a
// library that plainly holds something answers "I have never seen that".
func TestFloorKeepsBestWhenNothingIsComplete(t *testing.T) {
	tokens := []string{"module", "1", "ode"}
	got := Scored("2CSE Module 4 NS of ODE.docx", "", "", tokens)
	if got.Coverage < Floor(got.Coverage) {
		t.Fatal("best available fell below its own floor")
	}
}

func TestNameOutranksBody(t *testing.T) {
	tokens := []string{"turbine"}
	byName := Scored("Pelton Turbine.pdf", "", "", tokens)
	byBody := Scored("Lecture notes.pdf", "", "a passing mention of a turbine", tokens)
	if byName.Score <= byBody.Score {
		t.Fatalf("body mention outranked the title: name=%v body=%v", byName.Score, byBody.Score)
	}
}

// A term the question is about can sit anywhere in a document. Scanning only
// the opening made a lecture naming Bernoulli on its fifteenth slide
// unfindable by anything but its filename, which did not mention it either.
func TestContentIsSearchedBeyondTheOpening(t *testing.T) {
	deep := strings.Repeat("filler text about other things. ", 400) + " Bernoulli's equation "
	if len(deep) < 8000 {
		t.Fatalf("test text is only %d chars, not deep enough to be a test", len(deep))
	}
	got := Scored("M01 Introduction and Intelligent Agents.pptx", "", deep, []string{"bernoulli"})
	if got.Matched != 1 {
		t.Fatalf("did not find a term at character %d", strings.Index(deep, "Bernoulli"))
	}
}

func TestContainsWordKeepsWholeWordRule(t *testing.T) {
	if ContainsWord("a lecture on semiconductor diodes", "ode") {
		t.Fatal("ode matched inside diodes in body text")
	}
	if !ContainsWord("solving an ODE by separation", "ode") {
		t.Fatal("ode did not match itself in body text")
	}
	if !ContainsWord("see part1 of the notes", "1") {
		t.Fatal("digit did not match across the letter/digit boundary")
	}
	if !ContainsWord("first order differentials here", "differential") {
		t.Fatal("stem did not reach the plural in body text")
	}
	if ContainsWord("the diodes chapter", "die") {
		t.Fatal("short token matched by prefix in body text")
	}
}

func TestContainsWordIsCaseInsensitive(t *testing.T) {
	if !ContainsWord("MODULE I ELECTROCHEMISTRY", "electrochemistry") {
		t.Fatal("uppercase body text did not match a lowercase token")
	}
}

// A passing mention in the text must not rank level with a title that says
// exactly what was asked for. A pitch deck mentioning artificial intelligence,
// a question and a paper was offered as the answer to "question paper for AI".
func TestTitleCoverageBeatsScatteredMentions(t *testing.T) {
	tokens := []string{"question", "paper", "ai"}
	real := Scored("AI model question paper.pdf", "PYQ", "", tokens)
	deck := Scored("ReynaSIH.pptx", "Projects",
		"Reyna uses AI to answer any question about a paper you were sent", tokens)

	if real.Coverage != 1.0 {
		t.Fatalf("title match coverage = %v, want 1.0", real.Coverage)
	}
	if deck.Coverage >= real.Coverage {
		t.Fatalf("body mentions matched the title: deck=%v real=%v", deck.Coverage, real.Coverage)
	}
	if deck.Coverage >= Floor(real.Coverage) {
		t.Fatalf("deck survived the floor: coverage=%v floor=%v", deck.Coverage, Floor(real.Coverage))
	}
}

// But a content-only match must still win when it is all there is.
func TestContentOnlyStillSurvivesWhenNothingBetterExists(t *testing.T) {
	tokens := []string{"bernoulli"}
	only := Scored("M01 Introduction.pptx", "", "Bernoulli's equation appears here", tokens)
	if only.Coverage <= 0 {
		t.Fatal("content-only match scored nothing")
	}
	if only.Coverage < Floor(only.Coverage) {
		t.Fatal("the best available fell below its own floor")
	}
}

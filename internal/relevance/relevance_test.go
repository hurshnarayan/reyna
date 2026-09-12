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
// A title match heavily outranks scattered body mentions by Score.
func TestTitleScoreBeatsScatteredMentions(t *testing.T) {
	tokens := []string{"question", "paper", "ai"}
	real := Scored("AI model question paper.pdf", "PYQ", "", tokens)
	deck := Scored("ReynaSIH.pptx", "Projects",
		"Reyna uses AI to answer any question about a paper you were sent", tokens)

	if real.Coverage != 1.0 || deck.Coverage != 1.0 {
		t.Fatalf("both cover all query tokens: real=%v deck=%v", real.Coverage, deck.Coverage)
	}
	if real.Score <= deck.Score {
		t.Fatalf("title score must beat scattered body: real=%v deck=%v", real.Score, deck.Score)
	}
}

// WhatsApp numeric filenames (e.g. 4656526133.pdf) carry zero words in title.
// When all query tokens appear in the extracted body text, coverage must be 1.0
// and survive Floor so tickets, invoices, and scans are never discarded.
func TestNumericWhatsAppFilenameSurvivesFloor(t *testing.T) {
	tokens := []string{"departure", "hyderabad"}
	body := "Your departure time to Hyderabad Secunderabad is 22:00."
	ticket := Scored("4656526133.pdf", "", body, tokens)
	if ticket.Coverage != 1.0 {
		t.Fatalf("numeric WhatsApp file coverage = %v, want 1.0", ticket.Coverage)
	}
	if ticket.Coverage < Floor(1.0) {
		t.Fatalf("numeric WhatsApp file dropped by Floor(1.0): cover=%v", ticket.Coverage)
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

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}
	if sim := CosineSimilarity(a, b); sim < 0.999 || sim > 1.001 {
		t.Fatalf("identical vectors sim = %v, want 1.0", sim)
	}

	orthogonal := []float32{0.0, 1.0, 0.0}
	if sim := CosineSimilarity(a, orthogonal); sim < -0.001 || sim > 0.001 {
		t.Fatalf("orthogonal vectors sim = %v, want 0.0", sim)
	}

	opposite := []float32{-1.0, 0.0, 0.0}
	if sim := CosineSimilarity(a, opposite); sim < -1.001 || sim > -0.999 {
		t.Fatalf("opposite vectors sim = %v, want -1.0", sim)
	}

	empty := []float32{}
	if sim := CosineSimilarity(a, empty); sim != 0.0 {
		t.Fatalf("empty vector sim = %v, want 0.0", sim)
	}
}

func TestSummaryEntityOutranksGenericFilenameDecoy(t *testing.T) {
	primary := []string{"helmet", "cost"}
	allTokens := []string{"helmet", "cost", "price", "bill", "invoice", "receipt"}

	// Numeric WhatsApp file with AI summary indicating it is a helmet tax invoice
	realReceipt := ScoredWithPrimary("3144.pdf", "Invoices", "Tax invoice for FF818 STORM III helmet purchased by Harsh Narayan", "Total: 15400 INR", allTokens, primary)

	// Decoy laptop bill that merely has 'bill' in its filename
	laptopBill := ScoredWithPrimary("BILL-LAPTOP.pdf", "Invoices", "Tax invoice for Dell Laptop", "Total: 65000 INR", allTokens, primary)

	if realReceipt.Score <= laptopBill.Score {
		t.Fatalf("real helmet receipt (%v) should outrank generic laptop bill (%v)", realReceipt.Score, laptopBill.Score)
	}
}

// When a user asks for "ticket of harsh and khushi", a ticket that contains BOTH passengers
// and covers the query completely must outrank a decoy ticket that only contains one passenger.
func TestFullEntityCoverageOutranksPartialDecoy(t *testing.T) {
	primary := []string{"train", "ticket", "harsh", "khushi"}
	allTokens := []string{"train", "ticket", "harsh", "khushi", "booking", "travel"}

	// File 2181: Train ticket with both Harsh Narayan and Khushi Mehta
	twoPassengers := ScoredWithPrimary(
		"4656526133.pdf",
		"Di",
		"IRCTC Electronic Reservation Slip for Rajdhani Exp (22691) from KSR Bengaluru to Secunderabad.",
		"Electronic Reservation Slip (ERS)-Normal User Booked from KSR BENGALURU (SBC) To SECUNDERABAD JN (SC) Start Date* 12-Sept-2026 Train No./Name 22691/RAJDHANI EXP Passenger Details 1 KHUSHI MEHTA 2 HARSH NARAYAN Ticket Fare 3480 Booking Date 29-Aug-2026 Travel Insurance Premium 0.90",
		allTokens,
		primary,
	)

	// File 2180: Flight ticket with Khushi Mehta and others (Harsh is absent)
	onePassenger := ScoredWithPrimary(
		"44117890315459052945.pdf",
		"Di",
		"Flight booking confirmation for Khushi Mehta, Sonam Sahoo, Ramendra Sahoo, and Rishi Sahoo.",
		"E-Ticket booked with Booking Confirmed Booking ID : 44117890315459052945 Departure Flight: Hyderabad to Durgapur Sat, 17 Oct 2026 Mrs. Khushi Mehta Mrs. Sonam Sahoo Mr. Ramendra Sahoo Travel Insurance Covered",
		allTokens,
		primary,
	)

	if twoPassengers.Score <= onePassenger.Score {
		t.Fatalf("full entity match (%v) must outrank single passenger match (%v)", twoPassengers.Score, onePassenger.Score)
	}
}

func TestTypoMatchesIntendedWord(t *testing.T) {
	// "compriot" is a typo for "compatriot"
	tokens := []string{"compriot"}
	primary := []string{"compriot"}

	// Compatriot dictionary file
	compatriotDoc := ScoredWithPrimary(
		"IMG-20260910-WA0001.jpg",
		"Di",
		"A Google search result page defining the word 'compatriot' in English and Hindi.",
		"compatriot noun a person who comes from the same country as you",
		tokens,
		primary,
	)

	// Random electricity bill decoy
	billDoc := ScoredWithPrimary(
		"Electricity_Bill_June.pdf",
		"Bills",
		"Electricity bill for June 2026",
		"BESCOM electricity bill payment receipt",
		tokens,
		primary,
	)

	if compatriotDoc.Matched == 0 {
		t.Fatal("compriot should fuzzy match compatriot in summary/content")
	}
	if compatriotDoc.Score <= billDoc.Score {
		t.Fatalf("compatriot doc (%v) should outrank unrelated bill (%v)", compatriotDoc.Score, billDoc.Score)
	}
}




// Package relevance decides whether a document is actually about what someone
// asked, rather than merely containing the letters they typed.
//
// The rule this replaces was a substring test. "ode" is a substring of
// "diodes", so a question about ordinary differential equations returned a
// semiconductor lecture and cited it as the source. "1" is a substring of
// "part1", so a question about module 1 returned Module4_part1_Arrays. Both
// looked to the user like the assistant had understood nothing, because it
// had not.
//
// Matching here is on whole words. A filename is cut into words at every
// non-alphanumeric character and also at every letter/digit boundary, so
// "Module4_part1_Arrays.pptx" is the words module, 4, part, 1, arrays, pptx
// and "2CSE Module 1 ODE of first order.pdf" is 2, cse, module, 1, ode, of,
// first, order, pdf. "ode" is a word in the second and is not a word in
// "diodes", which is the whole distinction the old test could not draw.
package relevance

import (
	"math"
	"strings"
)

// minStemLen is the shortest token allowed to match by prefix.
//
// Prefix matching exists so "order" finds "ordering" and "differential" finds
// "differentials". Below five characters it stops being a stem and starts
// being a coincidence: "ode" would reach "odes" but also, with one more
// character of slack, the kind of accidental hit this package exists to stop.
const minStemLen = 5

// Words cuts text into the lowercase words it is built from.
//
// Splitting on the letter/digit boundary is what separates "part1" into part
// and 1. Without it every filename carrying any digit matches every question
// carrying any digit, which is how a question about module 1 reached a file
// about module 4.
func Words(text string) []string {
	var out []string
	var cur strings.Builder
	prevDigit := false
	started := false

	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
		started = false
	}

	for _, r := range strings.ToLower(text) {
		isLetter := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if !isLetter && !isDigit {
			flush()
			continue
		}
		if started && isDigit != prevDigit {
			flush()
		}
		cur.WriteRune(r)
		prevDigit = isDigit
		started = true
	}
	flush()
	return out
}

// Match reports whether token is one of these words.
//
// Exact first, then a prefix relation in either direction so singular and
// plural forms of the same word do not count as different subjects.
func Match(words []string, token string) bool {
	if token == "" {
		return false
	}
	for _, w := range words {
		if w == token {
			return true
		}
		if len(token) >= minStemLen && strings.HasPrefix(w, token) {
			return true
		}
		if len(w) >= minStemLen && strings.HasPrefix(token, w) {
			return true
		}
	}
	return false
}

// MatchText is Match against text that has not been split yet.
func MatchText(text, token string) bool { return Match(Words(text), token) }

// ContainsWord reports whether token appears anywhere in text as a whole word.
//
// Separate from Match because the two have different jobs. A filename is short
// enough to split into a slice of words; a document's text is not, and the
// whole of it has to be searched. An earlier revision scanned only the opening
// few thousand characters, on the reasoning that a title page says what a
// document is. It does, but that is an argument about ranking, not about
// finding: a lecture that names Bernoulli's equation on its fifteenth slide is
// still the file somebody asking about Bernoulli wants, and cutting the scan
// short made it unfindable by anything except its filename, which did not
// mention it either.
//
// Scans in place rather than building a word list, because the candidate set
// runs to a few hundred documents per question and some of them are thirty
// thousand characters.
func ContainsWord(text, token string) bool {
	if token == "" || len(token) > len(text) {
		return false
	}
	n, tl := len(text), len(token)
	for i := 0; i+tl <= n; i++ {
		if !equalFoldASCII(text[i:i+tl], token) {
			continue
		}
		if i > 0 && !boundary(text[i-1], text[i]) {
			continue
		}
		end := i + tl
		if end < n && !boundary(text[end-1], text[end]) {
			// Not a boundary, so the token sits inside a longer word. That is
			// allowed only as a stem: "differential" may reach
			// "differentials", while "ode" must never reach "diodes".
			if tl < minStemLen || !isLetter(text[end]) {
				continue
			}
		}
		return true
	}
	return false
}

// boundary reports whether a word ends between these two bytes.
//
// Either side being non-alphanumeric is the obvious case. The letter/digit
// transition is the one that matters here: it is what makes "1" a word inside
// "part1", and without it any text containing a digit matches any question
// containing a digit.
func boundary(prev, cur byte) bool {
	pa, ca := isAlnum(prev), isAlnum(cur)
	if !pa || !ca {
		return true
	}
	return isDigit(prev) != isDigit(cur)
}

func isDigit(b byte) bool  { return b >= '0' && b <= '9' }
func isLetter(b byte) bool { return (b|0x20) >= 'a' && (b|0x20) <= 'z' }
func isAlnum(b byte) bool  { return isDigit(b) || isLetter(b) }

// equalFoldASCII compares two byte strings ignoring ASCII case. The tokens are
// already lowercase; the document text is whatever the file contained.
func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca == cb {
			continue
		}
		if isLetter(ca) && isLetter(cb) && ca|0x20 == cb|0x20 {
			continue
		}
		return false
	}
	return true
}

// Result is how well one candidate answers a set of query tokens.
type Result struct {
	// Matched is how many query tokens appear as words in the candidate.
	Matched int

	// Coverage is Matched over the number of query tokens, so 1.0 means the
	// candidate accounts for everything that was asked. Coverage is what
	// decides whether a file is a plausible answer at all; Score only orders
	// the ones that are.
	Coverage float64

	// Adjacent is how many consecutive pairs of query tokens appear next to
	// each other, in order, in the candidate's name.
	//
	// This is what tells "Module 1" apart from "Module4_part1". Both contain
	// the words module and 1, so both cover the query completely, but only
	// one of them has them side by side, and that one is the file the person
	// meant.
	Adjacent int

	Score float64
}

// Scored reports how well a candidate matches, weighing the name heaviest.
//
// Name, then folder, then the text inside. A document's name is the strongest
// statement anyone makes about what it is, and body text is the weakest: a
// lecture that mentions module 1 once in a footnote is not about module 1.
func Scored(name, folder, content string, tokens []string) Result {
	var r Result
	if len(tokens) == 0 {
		return r
	}

	nameWords := Words(name)
	folderWords := Words(folder)

	// Coverage counts a word in the title for more than the same word buried in
	// the text.
	//
	// It used to count them the same, and coverage is what the floor cuts on,
	// so any document that happened to mention all the words anywhere scored as
	// complete a match as one whose name said exactly that. A pitch deck that
	// mentioned artificial intelligence, a question and a paper in passing
	// therefore ranked alongside "AI model question paper.pdf" and was offered
	// as an answer to a request for it.
	//
	// Body text still counts, at half. A document really about something says
	// so in the text and a content-only match is often all there is, so the
	// relative floor keeps those when nothing better exists; what it stops is a
	// passing mention standing level with a title.
	var credit float64
	for _, tok := range tokens {
		switch {
		case Match(nameWords, tok):
			r.Matched++
			credit += 1.0
			r.Score += 25
		case Match(folderWords, tok):
			r.Matched++
			credit += 1.0
			r.Score += 12
		case ContainsWord(content, tok):
			r.Matched++
			credit += 0.5
			r.Score += 4
		}
	}
	r.Coverage = credit / float64(len(tokens))

	for i := 0; i+1 < len(tokens); i++ {
		if adjacentIn(nameWords, tokens[i], tokens[i+1]) {
			r.Adjacent++
			r.Score += 40
		}
	}

	// The whole query appearing verbatim in the name is the strongest signal
	// available and outranks any accumulation of separate token hits.
	if len(tokens) > 1 && containsRun(nameWords, tokens) {
		r.Score += 60
	}
	return r
}

// adjacentIn reports whether a is immediately followed by b in words.
func adjacentIn(words []string, a, b string) bool {
	for i := 0; i+1 < len(words); i++ {
		if Match(words[i:i+1], a) && Match(words[i+1:i+2], b) {
			return true
		}
	}
	return false
}

// containsRun reports whether every token appears in order and unbroken.
func containsRun(words, tokens []string) bool {
	if len(tokens) == 0 || len(words) < len(tokens) {
		return false
	}
	for i := 0; i+len(tokens) <= len(words); i++ {
		ok := true
		for j, tok := range tokens {
			if !Match(words[i+j:i+j+1], tok) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// Floor is the coverage a candidate must reach to be treated as an answer,
// given the best coverage anything achieved.
//
// It is relative on purpose. If some file accounts for everything the person
// asked, files that account for less are not near misses, they are different
// documents, and showing them is what made a correct answer look like a
// guess. If nothing covers the query fully then the best available is the
// best there is, and cutting at an absolute threshold would answer "I have
// never seen that" about a library that plainly contains something.
func Floor(best float64) float64 {
	switch {
	case best >= 1.0:
		return 1.0
	case best <= 0:
		return 0
	default:
		// One token short of the best, so a three token query keeps its two
		// token matches when nothing matched all three.
		return best - 0.0001
	}
}

// CosineSimilarity computes the cosine similarity between two float32 vectors.
// Returns a score between -1.0 and 1.0 (typically 0.0 to 1.0 for normalized embeddings).
func CosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0.0
	}
	var dot, normA, normB float64
	for i := range a {
		va := float64(a[i])
		vb := float64(b[i])
		dot += va * vb
		normA += va * va
		normB += vb * vb
	}
	if normA <= 0 || normB <= 0 {
		return 0.0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

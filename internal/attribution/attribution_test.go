package attribution

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hurshnarayan/reyna/internal/model"
	"github.com/hurshnarayan/reyna/internal/whatsapp/export"
)

var utc = time.UTC

func at(y, mo, d, h, mi int) time.Time {
	return time.Date(y, time.Month(mo), d, h, mi, 0, 0, utc)
}

func ev(id int64, posted time.Time, sender, attachment string) Event {
	return Event{
		ID: id, ChatKey: "sem5", ChatName: "Sem 5 CS",
		SenderDisplay: sender, PostedAt: posted,
		AttachmentName: attachment, HasAttachment: true,
	}
}

// The user shared it themselves. Certain, and free.
func TestSelfSentIsCertain(t *testing.T) {
	r := Attribute(File{ID: 1, DiskName: "notes.pdf", MTime: at(2026, 8, 18, 21, 0), IsSent: true}, nil, utc)
	if r.Confidence() != 1.0 || r.Method() != model.AttrSelfSent {
		t.Fatalf("got %v/%v, want 1.0/self_sent", r.Confidence(), r.Method())
	}
	if !r.CanName() {
		t.Error("a file the user sent must be nameable")
	}
}

// An event naming the exact file beats everything else.
func TestExactNameWins(t *testing.T) {
	posted := at(2026, 8, 18, 21, 14)
	r := Attribute(
		File{ID: 1, DiskName: "Compiler_Notes.pdf", MTime: posted.Add(2 * time.Minute)},
		[]Event{
			ev(1, posted, "Priya", "Compiler_Notes.pdf"),
			ev(2, posted, "Rakesh", "Something_Else.pdf"),
		},
		utc,
	)
	if r.Best == nil || r.Best.EventID != 1 {
		t.Fatalf("matched the wrong event: %+v", r.Best)
	}
	if r.Confidence() != 0.95 {
		t.Errorf("confidence = %v, want 0.95", r.Confidence())
	}
}

// A file downloaded days later is still that file. A user who taps download on
// Friday for something posted Monday must not lose the sender.
func TestExactNameSurvivesDelay(t *testing.T) {
	posted := at(2026, 8, 18, 21, 14)
	r := Attribute(
		File{ID: 1, DiskName: "Compiler_Notes.pdf", MTime: posted.Add(72 * time.Hour)},
		[]Event{ev(1, posted, "Priya", "Compiler_Notes.pdf")},
		utc,
	)
	if r.Confidence() != 0.85 {
		t.Errorf("confidence = %v, want 0.85", r.Confidence())
	}
	if !r.CanName() {
		t.Error("still strong enough to name")
	}
}

// When WhatsApp renames the file the embedded date is all we have. One
// candidate that day is nameable; several is a guess.
func TestDateUniqueVersusAmbiguous(t *testing.T) {
	posted := at(2026, 8, 18, 21, 14)

	unique := Attribute(
		File{ID: 1, DiskName: "DOC-20260818-WA0007.pdf", MTime: posted.Add(30 * time.Minute)},
		[]Event{ev(1, posted, "Priya", "x.pdf")},
		utc,
	)
	if unique.Method() != model.AttrDateUnique || !unique.CanName() {
		t.Errorf("single candidate: got %v/%v, want date_unique and nameable",
			unique.Method(), unique.Confidence())
	}

	ambiguous := Attribute(
		File{ID: 1, DiskName: "DOC-20260818-WA0007.pdf", MTime: posted.Add(30 * time.Minute)},
		[]Event{
			ev(1, posted, "Priya", "x.pdf"),
			ev(2, at(2026, 8, 18, 9, 0), "Rakesh", "y.pdf"),
			ev(3, at(2026, 8, 18, 14, 0), "Mohit", "z.pdf"),
		},
		utc,
	)
	if ambiguous.CanName() {
		t.Error("three candidates on one day is a guess and must not be named")
	}
	if ambiguous.Method() != model.AttrTimeWindow {
		t.Errorf("method = %v, want time_window", ambiguous.Method())
	}
	if ambiguous.Confidence() < 0.30 {
		t.Error("still worth surfacing the chat")
	}
}

// Nothing matched is a legitimate answer, not an error to hide.
func TestNoCandidates(t *testing.T) {
	r := Attribute(
		File{ID: 1, DiskName: "DOC-20260818-WA0007.pdf", MTime: at(2026, 8, 18, 21, 14)},
		[]Event{ev(1, at(2026, 1, 1, 9, 0), "Priya", "unrelated.pdf")},
		utc,
	)
	if r.Best != nil || r.Confidence() != 0 || r.CanName() {
		t.Errorf("got %+v, want no attribution", r)
	}
}

// Every candidate is kept, so a later export can promote a better one without
// the earlier reasoning being lost.
func TestKeepsLosingCandidates(t *testing.T) {
	posted := at(2026, 8, 18, 21, 14)
	r := Attribute(
		File{ID: 1, DiskName: "DOC-20260818-WA0007.pdf", MTime: posted},
		[]Event{
			ev(1, posted, "Priya", "a.pdf"),
			ev(2, posted.Add(time.Minute), "Rakesh", "b.pdf"),
		},
		utc,
	)
	if len(r.Candidates) < 2 {
		t.Errorf("kept %d candidates, want the alternatives on record", len(r.Candidates))
	}
}

// The WhatsApp filename carries a date and an intra-day ordering.
func TestWhatsAppNameParsing(t *testing.T) {
	y, m, d, ok := WANameDate("DOC-20260818-WA0007.pdf")
	if !ok || y != 2026 || m != 8 || d != 18 {
		t.Errorf("got %d-%d-%d ok=%v", y, m, d, ok)
	}
	n, ok := WANameCounter("DOC-20260818-WA0007.pdf")
	if !ok || n != 7 {
		t.Errorf("counter = %d ok=%v, want 7", n, ok)
	}
	if _, _, _, ok := WANameDate("Compiler_Notes.pdf"); ok {
		t.Error("a user-named file has no embedded date")
	}
}

// The date comparison must use the zone it is given, not the machine's.
// WhatsApp stamps the filename with the phone's local date, so left implicit
// this silently mis-dates every file near midnight for anyone in another zone,
// and the symptom is a lost attribution rather than an error.
func TestDateComparisonUsesGivenZone(t *testing.T) {
	// 21:14 UTC on the 18th is 02:44 on the 19th in IST.
	ist := time.FixedZone("IST", 5*3600+1800)
	posted := at(2026, 8, 18, 21, 14)
	file := File{ID: 1, DiskName: "DOC-20260818-WA0007.pdf", MTime: posted}
	events := []Event{ev(1, posted, "Priya", "x.pdf")}

	if got := Attribute(file, events, utc); got.Method() != model.AttrDateUnique {
		t.Errorf("UTC: method = %v, want date_unique", got.Method())
	}
	if got := Attribute(file, events, ist); got.Method() == model.AttrDateUnique {
		t.Error("IST: the filename date is the 18th but the message falls on the 19th locally")
	}
}

// The rule the whole design exists to enforce: below the threshold, say where
// and when, never who.
func TestDescribeNeverNamesBelowThreshold(t *testing.T) {
	cases := []struct {
		conf         float64
		sender, chat string
		want         string
	}{
		{0.95, "Mohit", "Sem 5 CS", "Mohit · 18 August"},
		{0.45, "Mohit", "Sem 5 CS", "Sem 5 CS · 18 August"},
		{0.69, "Mohit", "Sem 5 CS", "Sem 5 CS · 18 August"},
		{0.0, "", "", "Found on your phone · 18 August"},
	}
	for _, c := range cases {
		if got := Describe(c.conf, c.sender, c.chat, "18 August"); got != c.want {
			t.Errorf("Describe(%v) = %q, want %q", c.conf, got, c.want)
		}
	}
}

// ── D8: accuracy against a real export ──

const sampleExport = `[15/08/2026, 09:12:44] Messages and calls are end-to-end encrypted.
[15/08/2026, 09:13:01] Mohit Sharma: guys anyone has the compiler notes
[15/08/2026, 09:15:22] Priya R: <attached: Compiler_Design_Module3.pdf>
[16/08/2026, 23:47:10] Rakesh: DOC-20260816-WA0007.pdf (file attached)
[17/08/2026, 08:02:00] Priya R: <attached: OS_Module_3.pdf>
[18/08/2026, 10:15:00] Mohit Sharma: <attached: DBMS_PYQ_2025.pdf>
[19/08/2026, 11:30:00] Ananya: <attached: Placement_DSA_Sheet.pdf>
[20/08/2026, 14:00:00] Rakesh: <attached: Syllabus_Sem5.pdf>
`

// The measurement the plan promised: score the join using only the signals the
// phone will actually have, against the export's own ground truth.
//
// This is the number that decides how the product behaves. If names survive on
// disk, attribution is near exact. If WhatsApp renames documents, the join
// falls back to date and ordering, and the chat import stops being a repair and
// becomes the primary path.
func TestAttributionAccuracyAgainstExport(t *testing.T) {
	parsed, err := export.ParseString(sampleExport, export.Options{
		DefaultDayFirst: true, Loc: utc,
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Attachments()) < 6 {
		t.Fatalf("fixture has %d attachments, expected 6", len(parsed.Attachments()))
	}

	// The two cases below are not variations on one number, they are two
	// different products, and the gap between them is the argument for the
	// chat import existing at all.

	t.Run("chat imported: events name the file", func(t *testing.T) {
		s := Simulate(parsed, SimulationOptions{EventsHaveFilenames: true, Zone: utc})
		t.Log("\n" + s.String())
		if s.Precision() < 0.99 || s.Recall() < 0.99 {
			t.Errorf("precision=%.2f recall=%.2f; an export states the answer outright",
				s.Precision(), s.Recall())
		}
	})

	t.Run("notifications only, filenames intact", func(t *testing.T) {
		s := Simulate(parsed, SimulationOptions{Zone: utc})
		t.Log("\n" + s.String())
		// A notification says someone sent a document, rarely which one, and a
		// human-chosen filename gives nothing away either. So there is almost
		// nothing to join on, and Reyna correctly goes quiet rather than
		// guessing. Low recall here is the honest outcome, not a defect.
		if s.Named > 0 && s.Precision() < 0.95 {
			t.Errorf("precision = %.2f; a wrong name is worse than no name", s.Precision())
		}
	})

	t.Run("notifications only, whatsapp renamed the files", func(t *testing.T) {
		s := Simulate(parsed, SimulationOptions{RenameToWhatsAppStyle: true, Zone: utc})
		t.Log("\n" + s.String())
		if s.Named > 0 && s.Precision() < 0.95 {
			t.Errorf("precision = %.2f", s.Precision())
		}
	})

	t.Run("downloaded a day late", func(t *testing.T) {
		s := Simulate(parsed, SimulationOptions{
			RenameToWhatsAppStyle: true,
			DownloadDelay:         26 * time.Hour,
			Zone:                  utc,
		})
		t.Log("\n" + s.String())
		if s.Named > 0 && s.Precision() < 0.95 {
			t.Errorf("precision = %.2f under delay", s.Precision())
		}
	})
}

// The measurement must be honest about its own setup: if the join could read
// attachment names off the events, it would be scoring the export against
// itself and would report a perfect result no matter how good the join is.
func TestSimulationWithholdsTheAnswer(t *testing.T) {
	parsed, err := export.ParseString(sampleExport, export.Options{DefaultDayFirst: true, Loc: utc})
	if err != nil {
		t.Fatal(err)
	}
	renamed := Simulate(parsed, SimulationOptions{RenameToWhatsAppStyle: true, Zone: utc})
	for method := range renamed.ByMethod {
		if method == model.AttrExport {
			t.Fatal("scored an exact-name match on renamed files; the simulation is leaking the answer")
		}
	}
}

// A real group shares several files a day, which is where date-based matching
// stops being able to pick one message. The clean fixture above has one file
// per day and flatters the result; this is the case that decides whether the
// chat import is a repair or the primary path.
const busyExport = `[18/08/2026, 09:15:00] Priya R: <attached: Compiler_Module1.pdf>
[18/08/2026, 11:30:00] Rakesh: <attached: Compiler_Module2.pdf>
[18/08/2026, 14:00:00] Mohit Sharma: <attached: OS_Notes.pdf>
[18/08/2026, 16:45:00] Ananya: <attached: DBMS_PYQ.pdf>
[19/08/2026, 10:00:00] Priya R: <attached: Placement_Sheet.pdf>
[19/08/2026, 10:30:00] Rakesh: <attached: Syllabus.pdf>
`

func TestAttributionOnABusyDay(t *testing.T) {
	parsed, err := export.ParseString(busyExport, export.Options{DefaultDayFirst: true, Loc: utc})
	if err != nil {
		t.Fatal(err)
	}

	imported := Simulate(parsed, SimulationOptions{EventsHaveFilenames: true, Zone: utc})
	t.Log("chat imported:\n" + imported.String())
	if imported.Precision() < 0.99 || imported.Recall() < 0.99 {
		t.Errorf("an export still states the answer outright: %.2f/%.2f",
			imported.Precision(), imported.Recall())
	}

	renamed := Simulate(parsed, SimulationOptions{RenameToWhatsAppStyle: true, Zone: utc})
	t.Log("notifications only, renamed, four files on one day:\n" + renamed.String())

	// This is the finding that matters. With several candidates on the same
	// day, date matching cannot pick one, so Reyna declines to name rather than
	// guessing. Recall collapses and precision holds, which is the trade the
	// design deliberately makes: a wrong name is worse than no name.
	if renamed.Named > 0 && renamed.Precision() < 0.95 {
		t.Errorf("precision = %.2f; naming the wrong person is the one unacceptable outcome",
			renamed.Precision())
	}
	if renamed.Recall() >= imported.Recall() {
		t.Errorf("recall without an export (%.2f) should be worse than with one (%.2f); "+
			"if it is not, the simulation is leaking the answer",
			renamed.Recall(), imported.Recall())
	}
}

func TestScoreStringIsReadable(t *testing.T) {
	s := Score{Files: 10, Named: 8, Correct: 7, Wrong: 1, Unnamed: 2,
		ByMethod: map[string]MethodScore{model.AttrDateUnique: {Named: 8, Correct: 7}}}
	out := s.String()
	for _, want := range []string{"precision", "recall", model.AttrDateUnique} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(fmt.Sprintf("%.1f", s.Precision()*100), "87.5") {
		t.Errorf("precision = %v, want 0.875", s.Precision())
	}
}

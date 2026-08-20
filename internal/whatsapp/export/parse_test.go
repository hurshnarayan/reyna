package export

import (
	"strings"
	"testing"
	"time"
)

func utc() Options { return Options{DefaultDayFirst: true, Loc: time.UTC} }

// TestParseHeaderFormats covers the timestamp shapes real exports produce.
// These differ by platform, OS version, locale and phone, and a pattern that
// handles only the one on the developer's own device silently drops every
// message from everyone else's.
func TestParseHeaderFormats(t *testing.T) {
	cases := []struct {
		name   string
		line   string
		want   time.Time
		sender string
		text   string
	}{
		{
			name:   "iOS bracketed with seconds",
			line:   "[12/08/2026, 21:14:03] Rahul Verma: hello",
			want:   time.Date(2026, 8, 12, 21, 14, 3, 0, time.UTC),
			sender: "Rahul Verma", text: "hello",
		},
		{
			name:   "Android dash separator",
			line:   "12/08/2026, 21:14 - Rahul Verma: hello",
			want:   time.Date(2026, 8, 12, 21, 14, 0, 0, time.UTC),
			sender: "Rahul Verma", text: "hello",
		},
		{
			name:   "two-digit year",
			line:   "[12/08/26, 21:14:03] Rahul: hello",
			want:   time.Date(2026, 8, 12, 21, 14, 3, 0, time.UTC),
			sender: "Rahul", text: "hello",
		},
		{
			name:   "12-hour PM",
			line:   "13/08/2026, 9:14 PM - Rahul: hello",
			want:   time.Date(2026, 8, 13, 21, 14, 0, 0, time.UTC),
			sender: "Rahul", text: "hello",
		},
		{
			name:   "12-hour AM midnight",
			line:   "13/08/2026, 12:05 AM - Rahul: hello",
			want:   time.Date(2026, 8, 13, 0, 5, 0, 0, time.UTC),
			sender: "Rahul", text: "hello",
		},
		{
			// iOS 17+ writes U+202F (narrow no-break space) before AM/PM. A
			// pattern using \s or a literal space fails on every message from a
			// recent iPhone.
			name:   "narrow no-break space before PM",
			line:   "[13/08/2026, 9:14:03 PM] Rahul: hello",
			want:   time.Date(2026, 8, 13, 21, 14, 3, 0, time.UTC),
			sender: "Rahul", text: "hello",
		},
		{
			// iOS prefixes lines with a left-to-right mark, which breaks any
			// anchored pattern.
			name:   "leading LTR mark",
			line:   "‎[13/08/2026, 21:14:03] Rahul: hello",
			want:   time.Date(2026, 8, 13, 21, 14, 3, 0, time.UTC),
			sender: "Rahul", text: "hello",
		},
		{
			name:   "dot separators",
			line:   "13.08.2026, 21:14 - Rahul: hello",
			want:   time.Date(2026, 8, 13, 21, 14, 0, 0, time.UTC),
			sender: "Rahul", text: "hello",
		},
		{
			name:   "sender name containing a colon",
			line:   "[13/08/2026, 21:14:03] Dr: Mehta: hello there",
			want:   time.Date(2026, 8, 13, 21, 14, 3, 0, time.UTC),
			sender: "Dr", text: "Mehta: hello there",
		},
		{
			// Splitting on the last colon would make the sender "check this out
			// (link" and lose the message.
			name:   "message body containing a colon",
			line:   "[13/08/2026, 21:14:03] Rahul: check this out: https://x.com",
			want:   time.Date(2026, 8, 13, 21, 14, 3, 0, time.UTC),
			sender: "Rahul", text: "check this out: https://x.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ParseString(tc.line, utc())
			if err != nil {
				t.Fatalf("ParseString: %v", err)
			}
			if len(res.Messages) != 1 {
				t.Fatalf("got %d messages (%d unparsed), want 1", len(res.Messages), res.UnparsedLines)
			}
			m := res.Messages[0]
			if !m.PostedAt.Equal(tc.want) {
				t.Errorf("PostedAt = %v, want %v", m.PostedAt, tc.want)
			}
			if m.Sender != tc.sender {
				t.Errorf("Sender = %q, want %q", m.Sender, tc.sender)
			}
			if m.Text != tc.text {
				t.Errorf("Text = %q, want %q", m.Text, tc.text)
			}
		})
	}
}

// TestParseAttachments covers both platforms' attachment notation, including
// non-English exports. Matching on the literal English "(file attached)" would
// drop every attachment from a Spanish or Hindi phone, which is precisely the
// data Reyna exists to attribute.
func TestParseAttachments(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantName string
		omitted  bool
	}{
		{
			name:     "iOS attached",
			line:     "[18/08/2026, 21:14:03] Mohit: <attached: DOC-20260818-WA0001.pdf>",
			wantName: "DOC-20260818-WA0001.pdf",
		},
		{
			name:     "Android file attached",
			line:     "18/08/2026, 21:14 - Mohit: DOC-20260818-WA0001.pdf (file attached)",
			wantName: "DOC-20260818-WA0001.pdf",
		},
		{
			name:     "Spanish localisation",
			line:     "18/08/2026, 21:14 - Mohit: DOC-20260818-WA0001.pdf (archivo adjunto)",
			wantName: "DOC-20260818-WA0001.pdf",
		},
		{
			name:     "Hindi localisation",
			line:     "18/08/2026, 21:14 - Mohit: DOC-20260818-WA0001.pdf (फ़ाइल संलग्न)",
			wantName: "DOC-20260818-WA0001.pdf",
		},
		{
			name:     "filename with spaces",
			line:     "[18/08/2026, 21:14:03] Mohit: <attached: Compiler Design Notes.pdf>",
			wantName: "Compiler Design Notes.pdf",
		},
		{
			name:    "media omitted",
			line:    "18/08/2026, 21:14 - Mohit: <Media omitted>",
			omitted: true,
		},
		{
			name:    "media omitted, Spanish",
			line:    "18/08/2026, 21:14 - Mohit: <Multimedia omitido>",
			omitted: true,
		},
		{
			// A sentence that happens to end in a parenthetical must not be
			// mistaken for an attachment.
			name: "plain message is not an attachment",
			line: "18/08/2026, 21:14 - Mohit: see you at 8 (bring notes.pdf)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ParseString(tc.line, utc())
			if err != nil {
				t.Fatalf("ParseString: %v", err)
			}
			if len(res.Messages) != 1 {
				t.Fatalf("got %d messages, want 1", len(res.Messages))
			}
			m := res.Messages[0]
			if m.AttachmentName != tc.wantName {
				t.Errorf("AttachmentName = %q, want %q", m.AttachmentName, tc.wantName)
			}
			if m.MediaOmitted != tc.omitted {
				t.Errorf("MediaOmitted = %v, want %v", m.MediaOmitted, tc.omitted)
			}
		})
	}
}

// TestDateOrderResolution pins the day/month decision. A single line cannot
// settle it, but one date above 12 anywhere in the file rules out that position
// as a month. Getting this wrong shifts every date by up to eleven months while
// looking entirely plausible.
func TestDateOrderResolution(t *testing.T) {
	t.Run("day above 12 proves day-first", func(t *testing.T) {
		in := "[13/08/2026, 10:00:00] A: x\n[05/08/2026, 10:00:00] A: y\n"
		res, err := ParseString(in, Options{DefaultDayFirst: false, Loc: time.UTC})
		if err != nil {
			t.Fatal(err)
		}
		if !res.DateOrder.DayFirst || !res.DateOrder.Proven {
			t.Fatalf("DateOrder = %v, want proven day-first", res.DateOrder)
		}
		// The default said month-first; the file overrode it.
		if got := res.Messages[0].PostedAt; got.Month() != time.August || got.Day() != 13 {
			t.Errorf("first message = %v, want 13 August", got)
		}
	})

	t.Run("second field above 12 proves month-first", func(t *testing.T) {
		in := "[08/13/2026, 10:00:00] A: x\n[08/05/2026, 10:00:00] A: y\n"
		res, err := ParseString(in, Options{DefaultDayFirst: true, Loc: time.UTC})
		if err != nil {
			t.Fatal(err)
		}
		if res.DateOrder.DayFirst || !res.DateOrder.Proven {
			t.Fatalf("DateOrder = %v, want proven month-first", res.DateOrder)
		}
		if got := res.Messages[0].PostedAt; got.Month() != time.August || got.Day() != 13 {
			t.Errorf("first message = %v, want 13 August", got)
		}
	})

	t.Run("ambiguous file falls back and says so", func(t *testing.T) {
		in := "[05/08/2026, 10:00:00] A: x\n[06/09/2026, 10:00:00] A: y\n"
		res, err := ParseString(in, Options{DefaultDayFirst: true, Loc: time.UTC})
		if err != nil {
			t.Fatal(err)
		}
		if res.DateOrder.Proven {
			t.Error("DateOrder claims to be proven; nothing in this file settles it")
		}
		if !res.DateOrder.DayFirst {
			t.Error("should have used the supplied default")
		}
		if !strings.Contains(res.DateOrder.String(), "assumed") {
			t.Errorf("String() = %q, should tell the caller it is an assumption", res.DateOrder)
		}
	})
}

// TestMultiLineMessages checks that continuation lines attach to the message
// they belong to rather than being dropped or counted as failures.
func TestMultiLineMessages(t *testing.T) {
	in := "[18/08/2026, 21:14:03] Mohit: first line\nsecond line\nthird line\n" +
		"[18/08/2026, 21:15:00] Priya: next message\n"
	res, err := ParseString(in, utc())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(res.Messages))
	}
	want := "first line\nsecond line\nthird line"
	if res.Messages[0].Text != want {
		t.Errorf("Text = %q, want %q", res.Messages[0].Text, want)
	}
	if res.UnparsedLines != 0 {
		t.Errorf("UnparsedLines = %d, want 0; continuations are not failures", res.UnparsedLines)
	}
}

// TestSystemMessages checks that WhatsApp's own notices are not attributed to a
// person. Reading "Messages and calls are end-to-end encrypted" as a sender
// would invent a contact and let it be cited as one.
func TestSystemMessages(t *testing.T) {
	in := "[18/08/2026, 21:14:03] Messages and calls are end-to-end encrypted. No one outside of this chat can read them.\n" +
		"[18/08/2026, 21:15:00] Mohit: real message\n" +
		"[18/08/2026, 21:16:00] Rahul Verma added Priya\n"
	res, err := ParseString(in, utc())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 3 {
		t.Fatalf("got %d messages, want 3", len(res.Messages))
	}
	if res.Messages[0].Sender != "" || !res.Messages[0].System {
		t.Errorf("encryption notice: sender=%q system=%v, want empty and system",
			res.Messages[0].Sender, res.Messages[0].System)
	}
	if res.Messages[1].Sender != "Mohit" {
		t.Errorf("real message sender = %q, want Mohit", res.Messages[1].Sender)
	}
	if res.Messages[2].Sender != "" {
		t.Errorf("group-add notice attributed to %q; it has no sender", res.Messages[2].Sender)
	}
}

// TestAttachmentsHelper checks the accessor Reyna actually consumes: the
// attachment-bearing subset, named or not. A media-omitted message still counts,
// because its sender and time can attribute a file that landed in the same chat
// at the same moment even though the export withheld the name.
func TestAttachmentsHelper(t *testing.T) {
	in := "[18/08/2026, 21:14:03] Mohit: <attached: notes.pdf>\n" +
		"[18/08/2026, 21:15:00] Priya: just chatting\n" +
		"[18/08/2026, 21:16:00] Rakesh: <Media omitted>\n"
	res, err := ParseString(in, utc())
	if err != nil {
		t.Fatal(err)
	}
	att := res.Attachments()
	if len(att) != 2 {
		t.Fatalf("got %d attachments, want 2", len(att))
	}
	if att[0].AttachmentName != "notes.pdf" || att[0].Sender != "Mohit" {
		t.Errorf("first = %q by %q", att[0].AttachmentName, att[0].Sender)
	}
	if !att[1].MediaOmitted || att[1].Sender != "Rakesh" {
		t.Errorf("second = omitted:%v by %q", att[1].MediaOmitted, att[1].Sender)
	}
}

// TestGarbageDoesNotKillTheParse checks the failure mode. One unrecognised line
// should cost one message, never the file: a user importing three years of
// history must not lose all of it to a single odd line.
func TestGarbageDoesNotKillTheParse(t *testing.T) {
	in := "total nonsense with no timestamp at the very top\n" +
		"[18/08/2026, 21:14:03] Mohit: good message\n" +
		"[99/99/9999, 99:99:99] Nobody: impossible date\n" +
		"[18/08/2026, 21:15:00] Priya: another good message\n"
	res, err := ParseString(in, utc())
	if err != nil {
		t.Fatalf("ParseString returned an error; malformed lines should be counted, not fatal: %v", err)
	}
	if len(res.Messages) != 2 {
		t.Fatalf("got %d messages, want the 2 good ones", len(res.Messages))
	}
	if res.UnparsedLines == 0 {
		t.Error("UnparsedLines = 0; bad lines must be reported, not silently dropped")
	}
}

// TestRealisticExport runs an end-to-end sample resembling an actual study-group
// export, and checks the thing Reyna ultimately needs: filename, sender, time.
func TestRealisticExport(t *testing.T) {
	in := `[15/08/2026, 09:12:44] Messages and calls are end-to-end encrypted.
[15/08/2026, 09:13:01] Mohit Sharma: guys anyone has the compiler notes
[15/08/2026, 09:15:22] Priya R: <attached: Compiler_Design_Module3.pdf>
[15/08/2026, 09:15:23] Priya R: here
[16/08/2026, 23:47:10] Rakesh: DOC-20260816-WA0007.pdf (file attached)
[16/08/2026, 23:47:55] Mohit Sharma: thanks
this is a second line of the same message
[17/08/2026, 08:02:00] Priya R: <Media omitted>
`
	res, err := ParseString(in, utc())
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if res.UnparsedLines != 0 {
		t.Errorf("UnparsedLines = %d, want 0", res.UnparsedLines)
	}
	if !res.DateOrder.DayFirst || !res.DateOrder.Proven {
		t.Errorf("DateOrder = %v, want proven day-first (the 15th, 16th and 17th settle it)", res.DateOrder)
	}

	att := res.Attachments()
	if len(att) != 3 {
		t.Fatalf("got %d attachments, want 3", len(att))
	}

	want := []struct {
		name   string
		sender string
		at     time.Time
	}{
		{"Compiler_Design_Module3.pdf", "Priya R", time.Date(2026, 8, 15, 9, 15, 22, 0, time.UTC)},
		{"DOC-20260816-WA0007.pdf", "Rakesh", time.Date(2026, 8, 16, 23, 47, 10, 0, time.UTC)},
		{"", "Priya R", time.Date(2026, 8, 17, 8, 2, 0, 0, time.UTC)},
	}
	for i, w := range want {
		if att[i].AttachmentName != w.name {
			t.Errorf("attachment %d name = %q, want %q", i, att[i].AttachmentName, w.name)
		}
		if att[i].Sender != w.sender {
			t.Errorf("attachment %d sender = %q, want %q", i, att[i].Sender, w.sender)
		}
		if !att[i].PostedAt.Equal(w.at) {
			t.Errorf("attachment %d time = %v, want %v", i, att[i].PostedAt, w.at)
		}
	}
}

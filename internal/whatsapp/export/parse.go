// Package export parses WhatsApp "Export chat" text files.
//
// This is Reyna's most reliable source of attribution. A file sitting in
// WhatsApp's media folder carries no record of who sent it or when it was
// posted; the export does, exactly, going back years. Notifications only cover
// the period since the user granted access and cannot be backfilled, so the
// export is what makes history attributable at all.
//
// Parsing is deliberately forgiving. Exports differ by platform, WhatsApp
// version, locale and phone, and a line we cannot parse should cost us one
// message rather than the file. Everything that fails to parse is counted and
// reported rather than silently dropped.
package export

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Message is one parsed line of an export.
type Message struct {
	// PostedAt is the message time as written in the export, interpreted in
	// Loc (see Options). Exports carry no timezone, so this is wall-clock time
	// in whatever zone the exporting phone was set to.
	PostedAt time.Time

	// Sender is the display name as WhatsApp rendered it: a contact name, or a
	// phone number for unsaved contacts. Empty for system messages.
	Sender string

	// Text is the message body, with continuation lines joined by newlines.
	Text string

	// AttachmentName is the filename when this message carried a file, e.g.
	// "DOC-20260818-WA0001.pdf". Empty otherwise.
	AttachmentName string

	// MediaOmitted marks a message that had an attachment the export did not
	// include (an export taken "without media"). The filename is unknown, but
	// the sender and time are not, which is enough to attribute a file that
	// arrived in the same chat at the same moment.
	MediaOmitted bool

	// System marks a message WhatsApp generated rather than a person: the
	// encryption notice, "X added Y", group subject changes.
	System bool

	// Line is the 1-based line number this message started on, for diagnostics.
	Line int
}

// HasAttachment reports whether this message carried a file, named or not.
func (m Message) HasAttachment() bool {
	return m.AttachmentName != "" || m.MediaOmitted
}

// Result is the outcome of parsing one export.
type Result struct {
	Messages []Message

	// DateOrder records how ambiguous dates were read, and whether that reading
	// was proven or assumed. An assumed order must be surfaced to the user,
	// because it silently shifts every date in the file.
	DateOrder DateOrder

	// UnparsedLines counts lines that began no message and continued none. A
	// nonzero count on a real export means a format we do not handle yet.
	UnparsedLines int
}

// Attachments returns only the messages that carried a file.
func (r Result) Attachments() []Message {
	var out []Message
	for _, m := range r.Messages {
		if m.HasAttachment() {
			out = append(out, m)
		}
	}
	return out
}

// DateOrder describes how the day and month fields were interpreted.
type DateOrder struct {
	DayFirst bool
	// Proven is true when the file itself settled the question: some date had a
	// first field above 12, which only day-first can explain. When false the
	// order came from Options.DefaultDayFirst and every date in the file is a
	// guess that may be wrong by up to eleven months.
	Proven bool
}

func (d DateOrder) String() string {
	order := "MM/DD"
	if d.DayFirst {
		order = "DD/MM"
	}
	if d.Proven {
		return order + " (proven by the file)"
	}
	return order + " (assumed)"
}

// Options controls parsing decisions the file cannot always settle.
type Options struct {
	// DefaultDayFirst is used when no date in the file disambiguates the order.
	// Day-first is the right default for the countries WhatsApp is largest in,
	// including India.
	DefaultDayFirst bool

	// Loc interprets the wall-clock times. Exports carry no timezone. Defaults
	// to time.Local.
	Loc *time.Location
}

func (o Options) loc() *time.Location {
	if o.Loc != nil {
		return o.Loc
	}
	return time.Local
}

// DefaultOptions is day-first, local time.
func DefaultOptions() Options {
	return Options{DefaultDayFirst: true, Loc: time.Local}
}

// headerRe matches the timestamp-and-sender prefix that begins every message.
//
// It has to absorb a lot of variation:
//
//	[12/08/2026, 21:14:03] Rahul Verma: text     iOS, bracketed, seconds
//	12/08/2026, 21:14 - Rahul Verma: text        Android, dash separator
//	[8/12/26, 9:14:03 PM] Rahul: text            12-hour, two-digit year
//	12.08.2026, 21:14 - Rahul: text              dot separators
//
// Seconds are optional, brackets are optional, the AM/PM marker is optional,
// and the date separator may be /, - or . — accepted independently per position
// rather than required to match, because Go's regexp engine (RE2) has no
// backreferences. Mixed separators are not a real export shape, so the extra
// permissiveness costs nothing.
//
// The AM/PM group allows any non-digit run before the marker rather than a
// literal space, because iOS 17 and later emit U+202F (narrow no-break space)
// there. A pattern written with \s or a literal space silently fails on every
// message from a recent iPhone.
var headerRe = regexp.MustCompile(
	`^\[?\s*(\d{1,4})[/.\-](\d{1,2})[/.\-](\d{2,4}),?\s+(\d{1,2}):(\d{2})(?::(\d{2}))?\s*(?:[^\d\w]?\s*([APap][._]?[Mm][._]?))?\s*\]?\s*(?:-\s*)?(.*)$`)

// attachedRe matches the iOS form: "<attached: NAME>". Some builds use
// non-ASCII angle quotes, so the delimiters are a character class.
var attachedRe = regexp.MustCompile(`[<‹]attached:\s*(.+?)\s*[>›]`)

// fileAttachedRe matches the Android form: "NAME (file attached)".
//
// The parenthetical is localised, so matching on its English text alone would
// drop every attachment from a non-English export. Instead the filename shape
// carries the match and the parenthetical only has to be *something* in
// brackets — which is what every localisation produces.
var fileAttachedRe = regexp.MustCompile(`^(\S.*?\.[A-Za-z0-9]{1,8})\s*[\(（]([^)）]{2,40})[\)）]\s*$`)

// mediaOmittedRe matches the placeholder left when an export excludes media:
// "<Media omitted>", "<Multimedia omitido>", "<Medien ausgelassen>" and so on.
//
// Matched by shape rather than by wording. The text is localised into every
// language WhatsApp ships, and a keyword list is a promise to keep chasing
// translations forever — one missing entry silently loses every attachment
// from that locale. What does not vary: the whole body is one short bracketed
// phrase carrying no filename. The named-attachment forms are tested first, so
// anything bracketed still reaching here is a placeholder.
var mediaOmittedRe = regexp.MustCompile(`^[<‹][^<>‹›]{1,60}[>›]$`)

// invisibleMarks are the direction and byte-order marks WhatsApp sprinkles
// through iOS exports. They sit at the start of lines and around timestamps,
// and any anchored pattern fails on them. Stripped before anything else runs.
var invisibleMarks = strings.NewReplacer(
	"\u200e", "", // LEFT-TO-RIGHT MARK
	"\u200f", "", // RIGHT-TO-LEFT MARK
	"\ufeff", "", // ZERO WIDTH NO-BREAK SPACE (BOM)
	"\u200b", "", // ZERO WIDTH SPACE
	"\u00a0", " ", // NO-BREAK SPACE
	"\u202f", " ", // NARROW NO-BREAK SPACE (iOS 17+, before AM/PM)
)

// systemPrefixes identify WhatsApp's own messages, which have no sender. Only
// used for lines that have no "Name: " prefix at all, so a person writing "You
// deleted this message" is not misread as a system notice.
var systemPrefixes = []string{
	"messages and calls are end-to-end encrypted",
	"messages to this chat and calls are now secured",
	"you created group",
	"you were added",
	"this message was deleted",
	"you deleted this message",
	"missed voice call",
	"missed video call",
	"changed the subject",
	"changed this group's icon",
	"added you",
	"security code changed",
	"disappearing messages",
	"joined using this group's invite link",
	"left",
}

// rawLine is a header-matched line before dates are resolved. Two passes are
// needed because the day/month order can only be decided after seeing every
// date in the file.
type rawLine struct {
	first, secondField, year int
	hour, minute, sec        int
	ampm                 string
	rest                 string
	lineNo               int
}

// Parse reads an export and returns its messages.
//
// Errors are returned only for I/O failures. A malformed line is counted in
// Result.UnparsedLines rather than failing the parse, because one unrecognised
// line should not cost the user their entire chat history.
func Parse(r io.Reader, opts Options) (*Result, error) {
	sc := bufio.NewScanner(r)
	// Exports contain pasted documents and long messages; the default 64KB
	// token limit is not enough.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	var (
		raws      []rawLine
		unparsed  int
		lineNo    int
		maxFirst  int
		maxSecond int
	)

	for sc.Scan() {
		lineNo++
		line := invisibleMarks.Replace(sc.Text())
		if strings.TrimSpace(line) == "" {
			continue
		}

		m := headerRe.FindStringSubmatch(line)
		if m == nil {
			// A continuation of the previous message. Multi-line messages have
			// no timestamp on lines after the first.
			if len(raws) > 0 {
				raws[len(raws)-1].rest += "\n" + line
			} else {
				unparsed++
			}
			continue
		}

		first := atoi(m[1])
		second := atoi(m[2])
		year := atoi(m[3])
		hour := atoi(m[4])
		minute := atoi(m[5])
		sec := atoi(m[6])

		// A four-digit leading field means the date is year-first
		// (2026-08-12), which is unambiguous.
		if first > 31 && len(m[1]) == 4 {
			year, first, second = first, second, year
		}

		if first > maxFirst {
			maxFirst = first
		}
		if second > maxSecond {
			maxSecond = second
		}

		raws = append(raws, rawLine{
				first: first, secondField: second, year: year,
			hour: hour, minute: minute, sec: sec,
			ampm: strings.ToUpper(strings.NewReplacer(".", "", "_", "").Replace(m[7])),
			rest: m[8], lineNo: lineNo,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read export: %w", err)
	}

	order := resolveDateOrder(maxFirst, maxSecond, opts.DefaultDayFirst)

	res := &Result{DateOrder: order, UnparsedLines: unparsed}
	for _, rl := range raws {
		msg, ok := buildMessage(rl, order, opts.loc())
		if !ok {
			res.UnparsedLines++
			continue
		}
		res.Messages = append(res.Messages, msg)
	}
	return res, nil
}

// ParseString is Parse over a string.
func ParseString(s string, opts Options) (*Result, error) {
	return Parse(strings.NewReader(s), opts)
}

// resolveDateOrder decides whether dates are day-first or month-first.
//
// A single line can never settle this: 08/12 is the 8th of December or the 12th
// of August depending on the phone. But across a whole export, any value above
// 12 in one position rules that position out as a month. If neither position
// ever exceeds 12 the file is genuinely ambiguous and the caller's default is
// used — recorded as unproven so the reading can be surfaced and corrected.
func resolveDateOrder(maxFirst, maxSecond int, defaultDayFirst bool) DateOrder {
	switch {
	case maxFirst > 12 && maxSecond <= 12:
		return DateOrder{DayFirst: true, Proven: true}
	case maxSecond > 12 && maxFirst <= 12:
		return DateOrder{DayFirst: false, Proven: true}
	default:
		// Either both exceed 12 (corrupt or mixed) or neither does (every date
		// in the file happens to fall in the first twelve days of a month).
		return DateOrder{DayFirst: defaultDayFirst, Proven: false}
	}
}

func buildMessage(rl rawLine, order DateOrder, loc *time.Location) (Message, bool) {
	day, month := rl.first, rl.secondField
	if !order.DayFirst {
		day, month = rl.secondField, rl.first
	}

	year := rl.year
	switch {
	case year < 100:
		// Two-digit year. WhatsApp did not exist before 2009, so a low value is
		// always this century.
		year += 2000
	case year < 1000:
		return Message{}, false
	}

	hour := rl.hour
	switch rl.ampm {
	case "PM":
		if hour < 12 {
			hour += 12
		}
	case "AM":
		if hour == 12 {
			hour = 0
		}
	}

	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || rl.minute > 59 || rl.sec > 59 {
		return Message{}, false
	}

	ts := time.Date(year, time.Month(month), day, hour, rl.minute, rl.sec, 0, loc)
	// time.Date normalises out-of-range days (31 April becomes 1 May). A date
	// that moved was not a real date.
	if ts.Day() != day || int(ts.Month()) != month {
		return Message{}, false
	}

	msg := Message{PostedAt: ts, Line: rl.lineNo}
	sender, body := splitSender(rl.rest)
	msg.Sender = sender
	msg.Text = body

	if sender == "" {
		msg.System = looksLikeSystemMessage(body)
	}

	if name, ok := findAttachment(body); ok {
		msg.AttachmentName = name
	} else if isMediaOmitted(body) {
		msg.MediaOmitted = true
	}
	return msg, true
}

// splitSender separates "Name: body" from a system line that has no sender.
//
// Splitting on the first ": " is deliberate. A display name can contain a
// colon, and message bodies contain them constantly ("link: https://…"), so
// splitting on the last or on any colon misattributes messages. The first
// separator is the only one WhatsApp itself guarantees.
//
// A candidate that looks like prose rather than a name is rejected, so a system
// line such as "You created group: Sem 5 CS" is not read as a person called
// "You created group".
func splitSender(rest string) (sender, body string) {
	idx := strings.Index(rest, ": ")
	if idx < 0 {
		// A sender with an empty message still ends in a colon.
		if t := strings.TrimRight(rest, " "); strings.HasSuffix(t, ":") {
			cand := strings.TrimSpace(strings.TrimSuffix(t, ":"))
			if plausibleSenderName(cand) {
				return cand, ""
			}
		}
		return "", rest
	}
	cand := strings.TrimSpace(rest[:idx])
	if !plausibleSenderName(cand) {
		return "", rest
	}
	return cand, rest[idx+2:]
}

// plausibleSenderName rejects candidates that are really the start of a system
// message. Display names are short and are not sentences; WhatsApp's own
// notices are longer and read as prose.
func plausibleSenderName(s string) bool {
	if s == "" || len(s) > 80 {
		return false
	}
	if strings.ContainsAny(s, "\n\t") {
		return false
	}
	lower := strings.ToLower(s)
	for _, p := range systemPrefixes {
		if strings.HasPrefix(lower, p) || strings.Contains(lower, p) {
			return false
		}
	}
	// Contact names are rarely more than a few words. Anything longer is prose.
	if len(strings.Fields(s)) > 6 {
		return false
	}
	return true
}

func looksLikeSystemMessage(body string) bool {
	lower := strings.ToLower(strings.TrimSpace(body))
	for _, p := range systemPrefixes {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// findAttachment extracts the filename from either platform's attachment
// notation.
func findAttachment(body string) (string, bool) {
	trimmed := strings.TrimSpace(body)

	if m := attachedRe.FindStringSubmatch(trimmed); m != nil {
		if name := strings.TrimSpace(m[1]); name != "" {
			return name, true
		}
	}

	// Android: "NAME (file attached)". The parenthetical is localised, so the
	// filename shape does the work and the bracketed text only has to exist.
	// Guard against a plain sentence that happens to end in a parenthetical by
	// requiring the candidate to look like a filename.
	if m := fileAttachedRe.FindStringSubmatch(trimmed); m != nil {
		name := strings.TrimSpace(m[1])
		if looksLikeFilename(name) {
			return name, true
		}
	}
	return "", false
}

// looksLikeFilename keeps "IMG-20260818-WA0001.jpg (file attached)" while
// rejecting "see you at 8 (bring notes.pdf)".
func looksLikeFilename(s string) bool {
	if s == "" || len(s) > 260 {
		return false
	}
	dot := strings.LastIndex(s, ".")
	if dot <= 0 || dot == len(s)-1 {
		return false
	}
	for _, r := range s[dot+1:] {
		if !isASCIILetterOrDigit(r) {
			return false
		}
	}
	// Real attachment names do not read as sentences.
	return len(strings.Fields(s)) <= 8
}

func isASCIILetterOrDigit(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// isMediaOmitted reports whether the body is a placeholder for an attachment
// the export left out. Callers must test the named-attachment forms first;
// this only decides whether a bracketed phrase with no filename in it is a
// media placeholder, which by then is the only thing it can be.
func isMediaOmitted(body string) bool {
	t := strings.TrimSpace(body)
	if !mediaOmittedRe.MatchString(t) {
		return false
	}
	// A bracketed phrase that names a file is an attachment we failed to parse,
	// not an omission. Better to report nothing than to record the wrong thing.
	// The brackets may be multibyte, so strip by rune.
	runes := []rune(t)
	inner := strings.TrimSpace(string(runes[1 : len(runes)-1]))
	return !looksLikeFilename(inner)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

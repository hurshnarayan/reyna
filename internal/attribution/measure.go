package attribution

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hurshnarayan/reyna/internal/model"
	"github.com/hurshnarayan/reyna/internal/whatsapp/export"
)

// Measuring how good on-device attribution actually is.
//
// A chat export is ground truth: it states the true sender and time for every
// attachment. So accuracy can be scored without a device and without the
// Baileys bot that used to provide it, by taking a real export, deriving the
// true answers, and then re-running the join using only the signals the phone
// will actually have.
//
// That last part is the whole point. Feeding the join the export's own
// attachment names would score the export against itself and report a hundred
// percent. The simulation deliberately withholds them, leaving what a file on
// disk really carries: a name, a modification time, and whether it sat under
// /Sent/.

// Score is the outcome of one simulation.
type Score struct {
	Files int

	// Named is how many the join felt able to attribute at or above the
	// threshold. Correct and Wrong split those by whether it was right.
	Named   int
	Correct int
	Wrong   int

	// Unnamed is how many it declined to name. Not an error: saying "shared in
	// Sem 5 CS" when the sender is genuinely unknowable is the correct answer,
	// and the design prefers it to a guess.
	Unnamed int

	ByMethod map[string]MethodScore
}

type MethodScore struct {
	Named   int
	Correct int
}

// Precision is how often a stated name was right.
//
// The number that matters. A wrong name is worse than no name, because the
// user cannot tell it is wrong and will act on it.
func (s Score) Precision() float64 {
	if s.Named == 0 {
		return 0
	}
	return float64(s.Correct) / float64(s.Named)
}

// Recall is how often a knowable sender was actually stated.
func (s Score) Recall() float64 {
	if s.Files == 0 {
		return 0
	}
	return float64(s.Correct) / float64(s.Files)
}

func (s Score) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "files=%d named=%d correct=%d wrong=%d unnamed=%d\n",
		s.Files, s.Named, s.Correct, s.Wrong, s.Unnamed)
	fmt.Fprintf(&b, "precision=%.1f%% recall=%.1f%%\n",
		s.Precision()*100, s.Recall()*100)

	methods := make([]string, 0, len(s.ByMethod))
	for m := range s.ByMethod {
		methods = append(methods, m)
	}
	sort.Strings(methods)
	for _, m := range methods {
		ms := s.ByMethod[m]
		p := 0.0
		if ms.Named > 0 {
			p = float64(ms.Correct) / float64(ms.Named) * 100
		}
		fmt.Fprintf(&b, "  %-14s named=%-4d correct=%-4d precision=%.0f%%\n", m, ms.Named, ms.Correct, p)
	}
	return b.String()
}

// SimulationOptions controls how faithfully the on-device situation is
// reproduced.
type SimulationOptions struct {
	// RenameToWhatsAppStyle replaces every filename with DOC-YYYYMMDD-WAnnnn,
	// which is what WhatsApp does to some documents. This is the highest-value
	// unknown in the whole design, so it is a switch rather than an assumption:
	// running the simulation both ways brackets the real answer.
	RenameToWhatsAppStyle bool

	// EventsHaveFilenames models whether the phone has imported a chat export.
	//
	// This is the single biggest lever on the result and the two cases are
	// completely different products. An export states the filename for every
	// message, so the exact-name rule fires and attribution is near perfect. A
	// notification almost never names the file, so the join is left with time
	// and whatever the filename itself gives away.
	EventsHaveFilenames bool

	// DownloadDelay shifts each file's mtime after its message, standing in for
	// a user who taps download later. Zero means auto-download.
	DownloadDelay time.Duration

	Zone *time.Location
}

// Simulate scores the join against an export's own ground truth.
//
// Events are taken from the export, then each attachment is turned into a file
// as it would appear on disk, with the export's attachment name withheld from
// the event so the join cannot simply read off the answer.
func Simulate(result *export.Result, opts SimulationOptions) Score {
	zone := opts.Zone
	if zone == nil {
		zone = time.Local
	}

	attachments := result.Attachments()
	score := Score{ByMethod: map[string]MethodScore{}}

	// Events as the phone would hold them. Whether they carry a filename is
	// exactly the difference between having imported a chat export and having
	// only watched notifications, and it dominates the result.
	events := make([]Event, 0, len(attachments))
	for i, m := range attachments {
		e := Event{
			ID:            int64(i + 1),
			ChatKey:       "chat",
			ChatName:      "Chat",
			SenderDisplay: m.Sender,
			PostedAt:      m.PostedAt,
			HasAttachment: true,
		}
		if opts.EventsHaveFilenames {
			e.AttachmentName = m.AttachmentName
			e.Source = model.AttrExport
		}
		events = append(events, e)
	}

	for i, m := range attachments {
		if m.AttachmentName == "" || m.Sender == "" {
			// Nothing to match, or nothing to check against.
			continue
		}
		score.Files++

		diskName := m.AttachmentName
		if opts.RenameToWhatsAppStyle {
			diskName = fmt.Sprintf("DOC-%s-WA%04d%s",
				m.PostedAt.In(zone).Format("20060102"), i%10000, extOf(m.AttachmentName))
		}

		f := File{
			ID:       int64(i + 1),
			DiskName: diskName,
			MTime:    m.PostedAt.Add(opts.DownloadDelay),
		}

		r := Attribute(f, events, zone)
		if !r.CanName() {
			score.Unnamed++
			continue
		}

		score.Named++
		method := r.Method()
		ms := score.ByMethod[method]
		ms.Named++

		// Right when the winning event is the message this file really came
		// from. Compared by sender rather than event id, because two messages
		// from the same person are interchangeable for attribution purposes.
		var got string
		if r.Best != nil {
			for _, e := range events {
				if e.ID == r.Best.EventID {
					got = e.SenderDisplay
					break
				}
			}
		}
		if strings.EqualFold(got, m.Sender) {
			score.Correct++
			ms.Correct++
		} else {
			score.Wrong++
		}
		score.ByMethod[method] = ms
	}
	return score
}

func extOf(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i:]
	}
	return ""
}

// Confirm the threshold has not drifted from the shared constant.
var _ = model.AttributionMinNamed

package docs

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// buildZip writes a minimal Office archive: real zip container, real XML,
// exercising the same reader path a file from Word or PowerPoint takes.
func buildZip(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDocxKeepsParagraphsAndTables(t *testing.T) {
	data := buildZip(t, map[string]string{
		"word/document.xml": `<?xml version="1.0"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
 <w:body>
  <w:p><w:r><w:t>Module 4: Numerical Solution of ODE</w:t></w:r></w:p>
  <w:p><w:r><w:t>Euler method</w:t></w:r><w:r><w:tab/></w:r><w:r><w:t>step size h</w:t></w:r></w:p>
  <w:p/>
  <w:p><w:r><w:t xml:space="preserve">Runge-Kutta </w:t></w:r><w:r><w:t>fourth order</w:t></w:r></w:p>
 </w:body>
</w:document>`,
	})

	got, err := Text("2CSE Module 4 NS of ODE.docx", data)
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	for _, want := range []string{
		"Module 4: Numerical Solution of ODE",
		"Euler method\tstep size h",
		// Two runs inside one paragraph are one line, not two. Splitting them
		// breaks quote verification, which matches a citation against the
		// stored text character for character.
		"Runge-Kutta fourth order",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestPptxNumbersSlidesInReadingOrder(t *testing.T) {
	slide := func(text string) string {
		return `<?xml version="1.0"?>
<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"
       xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
 <p:cSld><p:spTree><p:sp><p:txBody>
   <a:p><a:r><a:t>` + text + `</a:t></a:r></a:p>
 </p:txBody></p:sp></p:spTree></p:cSld>
</p:sld>`
	}
	data := buildZip(t, map[string]string{
		"ppt/slides/slide1.xml":  slide("Title slide"),
		"ppt/slides/slide2.xml":  slide("Second slide"),
		"ppt/slides/slide10.xml": slide("Tenth slide"),
	})

	got, err := Text("deck.pptx", data)
	if err != nil {
		t.Fatalf("Text: %v", err)
	}

	// Slide 10 sorts before slide 2 under a string comparison, which put a
	// deck's pages in the wrong order and sent a citation to the wrong slide.
	wantOrder := []string{"[[page 1]]", "Title slide", "[[page 2]]", "Second slide", "[[page 3]]", "Tenth slide"}
	at := 0
	for _, w := range wantOrder {
		i := strings.Index(got[at:], w)
		if i < 0 {
			t.Fatalf("expected %q after position %d, got:\n%s", w, at, got)
		}
		at += i + len(w)
	}
}

func TestXlsxResolvesSharedStringsIntoRows(t *testing.T) {
	data := buildZip(t, map[string]string{
		"xl/sharedStrings.xml": `<?xml version="1.0"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
 <si><t>Subject</t></si><si><t>Room</t></si><si><t>Operating Systems</t></si><si><t>B-207</t></si>
</sst>`,
		"xl/worksheets/sheet1.xml": `<?xml version="1.0"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
 <sheetData>
  <row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>
  <row r="2"><c r="A2" t="s"><v>2</v></c><c r="B2" t="s"><v>3</v></c></row>
 </sheetData>
</worksheet>`,
	})

	got, err := Text("timetable.xlsx", data)
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	// A cell holds an index into the string table, not the text. Storing the
	// index is how a room number becomes the number 3.
	if !strings.Contains(got, "Operating Systems | B-207") {
		t.Errorf("shared strings not resolved into the row:\n%s", got)
	}
}

func TestPlainAndHTML(t *testing.T) {
	if got, _ := Text("notes.md", []byte("# Heading\n\n\n\nbody")); got != "# Heading\n\nbody" {
		t.Errorf("markdown: blank runs not collapsed: %q", got)
	}
	got, err := Text("page.html", []byte(
		`<html><head><style>p{color:red}</style><script>var x=1</script></head><body><p>Room B-207</p></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Room B-207") {
		t.Errorf("html text lost: %q", got)
	}
	for _, leaked := range []string{"color:red", "var x"} {
		if strings.Contains(got, leaked) {
			t.Errorf("style or script contents leaked into the text: %q", got)
		}
	}
}

func TestUnsupportedAndBrokenInputsFailCleanly(t *testing.T) {
	if CanExtract("scan.pdf") {
		t.Error("a PDF needs the model, it must not claim a local reader")
	}
	if CanExtract("holiday.jpg") {
		t.Error("images have no text to read")
	}
	// A truncated upload must be an error, never a partial reading passed off
	// as the document.
	if _, err := Text("broken.docx", []byte("PK\x03\x04 not really a zip")); err == nil {
		t.Error("expected an error for a corrupt archive")
	}
}

func TestClipNeverSplitsARune(t *testing.T) {
	long := strings.Repeat("é", MaxChars)
	got := clip(long)
	if len(got) > MaxChars {
		t.Errorf("clip returned %d bytes, over the %d limit", len(got), MaxChars)
	}
	if !strings.HasSuffix(got, "é") {
		t.Error("clip cut a multi-byte character in half")
	}
}

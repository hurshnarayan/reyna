// Package docs pulls readable text out of a document without a model call and
// without OCR.
//
// It exists because the only document type Gemini reads natively is PDF. A
// .docx or .pptx sent as inline data is refused, and the code that used to
// handle that fell back to "infer what this document probably contains from
// its filename", which is a guess presented as a reading. A slide deck called
// "Module 2 (3).docx" produced a confident paragraph about organisational
// communication that appeared nowhere in the file.
//
// Office formats are zip archives of XML, so the text is right there. Reading
// it locally is exact, instant, costs nothing against a small daily model
// allowance, and needs nothing beyond the standard library, which matters in a
// backend that has two dependencies in total.
package docs

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxChars bounds what is kept from any one document.
//
// Long enough for a lecture deck or a contract, short enough that a thousand
// page manual cannot fill the database or the model's context on its own.
const MaxChars = 24000

// CanExtract reports whether Text can read this file without a model.
func CanExtract(fileName string) bool {
	switch strings.ToLower(path.Ext(fileName)) {
	case ".docx", ".pptx", ".xlsx",
		".txt", ".md", ".csv", ".log", ".json", ".xml", ".html", ".htm":
		return true
	}
	return false
}

// Text returns the readable text of a document, or an error if this format
// needs something else to read it.
//
// Pages are marked with [[page N]] on their own line wherever the format has a
// real notion of a page: one per slide in a deck, one per sheet in a workbook.
// The marker is the same one the citation code counts to say which page an
// answer came from, so a quote from slide six lands on slide six.
func Text(fileName string, data []byte) (string, error) {
	switch strings.ToLower(path.Ext(fileName)) {
	case ".docx":
		return zipXMLText(data, docx)
	case ".pptx":
		return zipXMLText(data, pptx)
	case ".xlsx":
		return xlsxText(data)
	case ".txt", ".md", ".csv", ".log", ".json", ".xml":
		return clip(sanitise(string(data))), nil
	case ".html", ".htm":
		return clip(stripMarkup(string(data))), nil
	}
	return "", fmt.Errorf("docs: no local reader for %q", path.Ext(fileName))
}

// ── Office XML ──

// layout describes where the text lives in one of the Office formats.
type layout struct {
	// parts selects the XML entries to read, in the order returned.
	parts func(*zip.Reader) []*zip.File
	// text is the element holding a run of characters.
	text string
	// para ends a line.
	para string
	// pagePerPart writes a [[page N]] marker before each part.
	pagePerPart bool
}

var docx = layout{
	parts: func(r *zip.Reader) []*zip.File {
		return pick(r, func(n string) bool { return n == "word/document.xml" })
	},
	text: "t",
	para: "p",
}

var pptx = layout{
	parts: func(r *zip.Reader) []*zip.File {
		s := pick(r, func(n string) bool {
			return strings.HasPrefix(n, "ppt/slides/slide") && strings.HasSuffix(n, ".xml")
		})
		// slide2 must not sort after slide10, which is what a plain string
		// comparison does and why decks came out with their pages shuffled.
		sort.Slice(s, func(i, j int) bool { return slideNo(s[i].Name) < slideNo(s[j].Name) })
		return s
	},
	text:        "t",
	para:        "p",
	pagePerPart: true,
}

func zipXMLText(data []byte, l layout) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("docs: not a readable archive: %w", err)
	}
	parts := l.parts(r)
	if len(parts) == 0 {
		return "", fmt.Errorf("docs: archive holds no text parts")
	}

	var out strings.Builder
	for i, f := range parts {
		if l.pagePerPart {
			fmt.Fprintf(&out, "[[page %d]]\n", i+1)
		}
		body, err := readAll(f)
		if err != nil {
			continue
		}
		if err := runsToLines(&out, body, l.text, l.para); err != nil {
			continue
		}
		if out.Len() >= MaxChars {
			break
		}
	}
	return clip(out.String()), nil
}

// runsToLines walks the XML and writes one line per paragraph.
//
// Namespaces are matched on local name alone. The prefixes differ between
// formats and between the tools that wrote the file, and matching the full
// name meant a deck written by one program read fine and the same deck
// re-saved by another read as empty.
func runsToLines(out *strings.Builder, body []byte, textEl, paraEl string) error {
	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.Strict = false
	inText := 0
	var line strings.Builder

	flush := func() {
		s := strings.TrimRight(line.String(), " \t")
		line.Reset()
		if s == "" {
			return
		}
		out.WriteString(s)
		out.WriteByte('\n')
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case textEl:
				inText++
			case "tab":
				line.WriteByte('\t')
			case "br", "cr":
				flush()
			}
		case xml.EndElement:
			switch t.Name.Local {
			case textEl:
				if inText > 0 {
					inText--
				}
			case paraEl:
				flush()
			}
		case xml.CharData:
			if inText > 0 {
				line.Write(t)
			}
		}
		if out.Len() >= MaxChars {
			break
		}
	}
	flush()
	return nil
}

// ── Spreadsheets ──

// xlsxText renders a workbook as one line per row, columns joined by " | ".
//
// The same shape the extraction prompt asks a model to produce for a table,
// so a quote taken from a row matches whether the row came from a spreadsheet
// or from a table inside a PDF.
func xlsxText(data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("docs: not a readable archive: %w", err)
	}

	shared := sharedStrings(r)
	sheets := pick(r, func(n string) bool {
		return strings.HasPrefix(n, "xl/worksheets/sheet") && strings.HasSuffix(n, ".xml")
	})
	sort.Slice(sheets, func(i, j int) bool { return slideNo(sheets[i].Name) < slideNo(sheets[j].Name) })
	if len(sheets) == 0 {
		return "", fmt.Errorf("docs: workbook holds no sheets")
	}

	var out strings.Builder
	for i, f := range sheets {
		fmt.Fprintf(&out, "[[page %d]]\n", i+1)
		body, err := readAll(f)
		if err != nil {
			continue
		}
		writeSheet(&out, body, shared)
		if out.Len() >= MaxChars {
			break
		}
	}
	return clip(out.String()), nil
}

func writeSheet(out *strings.Builder, body []byte, shared []string) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.Strict = false

	var row []string
	var cell strings.Builder
	inValue, sharedRef := false, false

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "c":
				cell.Reset()
				sharedRef = false
				for _, a := range t.Attr {
					if a.Name.Local == "t" && a.Value == "s" {
						sharedRef = true
					}
				}
			case "v", "t":
				inValue = true
			}
		case xml.CharData:
			if inValue {
				cell.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v", "t":
				inValue = false
			case "c":
				v := strings.TrimSpace(cell.String())
				if sharedRef {
					if i, err := strconv.Atoi(v); err == nil && i >= 0 && i < len(shared) {
						v = shared[i]
					}
				}
				row = append(row, v)
			case "row":
				if line := strings.Trim(strings.Join(row, " | "), " |"); line != "" {
					out.WriteString(line)
					out.WriteByte('\n')
				}
				row = row[:0]
			}
		}
		if out.Len() >= MaxChars {
			return
		}
	}
}

// sharedStrings is the workbook's string table. Every text cell is an index
// into it rather than the text itself.
func sharedStrings(r *zip.Reader) []string {
	files := pick(r, func(n string) bool { return n == "xl/sharedStrings.xml" })
	if len(files) == 0 {
		return nil
	}
	body, err := readAll(files[0])
	if err != nil {
		return nil
	}
	var out []string
	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.Strict = false
	var cur strings.Builder
	inText, inItem := false, false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				inItem = true
				cur.Reset()
			case "t":
				inText = true
			}
		case xml.CharData:
			if inItem && inText {
				cur.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "si":
				out = append(out, cur.String())
				inItem = false
			}
		}
	}
	return out
}

// ── helpers ──

func pick(r *zip.Reader, want func(string) bool) []*zip.File {
	var out []*zip.File
	for _, f := range r.File {
		if want(f.Name) {
			out = append(out, f)
		}
	}
	return out
}

func readAll(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	// Bounded, because a zip entry declares its own size and a hostile or
	// broken file can declare a very large one.
	return io.ReadAll(io.LimitReader(rc, 32<<20))
}

// slideNo pulls the trailing number out of names like "ppt/slides/slide10.xml"
// so parts sort in the order a reader would see them.
func slideNo(name string) int {
	base := strings.TrimSuffix(path.Base(name), path.Ext(name))
	i := len(base)
	for i > 0 && base[i-1] >= '0' && base[i-1] <= '9' {
		i--
	}
	n, err := strconv.Atoi(base[i:])
	if err != nil {
		return 0
	}
	return n
}

// stripMarkup removes tags, and the script and style blocks whose contents are
// not prose, from an HTML document.
func stripMarkup(s string) string {
	for _, tag := range []string{"script", "style"} {
		for {
			open := strings.Index(strings.ToLower(s), "<"+tag)
			if open < 0 {
				break
			}
			close := strings.Index(strings.ToLower(s[open:]), "</"+tag+">")
			if close < 0 {
				s = s[:open]
				break
			}
			s = s[:open] + s[open+close+len(tag)+3:]
		}
	}
	var out strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>':
			if depth > 0 {
				depth--
			}
			out.WriteByte('\n')
		case depth == 0:
			out.WriteRune(r)
		}
	}
	return sanitise(out.String())
}

// sanitise drops invalid UTF-8 and collapses runs of blank lines, so stored
// text stays valid in the database and readable in a citation.
func sanitise(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, l := range lines {
		l = strings.TrimRight(l, " \t\r")
		if strings.TrimSpace(l) == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func clip(s string) string {
	s = sanitise(s)
	if len(s) <= MaxChars {
		return s
	}
	s = s[:MaxChars]
	// Never cut a rune in half; the result is stored and later matched
	// character for character against a quote.
	for !utf8.ValidString(s) && len(s) > 0 {
		s = s[:len(s)-1]
	}
	return s
}

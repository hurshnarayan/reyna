package nlp

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
)

// ExtractOfficeText pulls plain text out of a DOCX, PPTX, or XLSX file using
// only the standard library. Office documents are zip archives containing XML
// — we walk the archive, find the parts that hold prose, strip the XML
// markup, and return concatenated text. Capped at maxBytes to keep the
// downstream prompt small.
//
// Used by the classifier to give Gemini real document content for non-PDF
// files instead of guessing from the filename.
func ExtractOfficeText(data []byte, mimeType string, maxBytes int) (string, error) {
	if maxBytes <= 0 {
		maxBytes = 50 * 1024
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}

	// Decide which inner XML files to read based on the document type.
	var keep func(name string) bool
	switch {
	case strings.Contains(mimeType, "wordprocessingml") || strings.HasSuffix(mimeType, "docx"):
		keep = func(name string) bool {
			return name == "word/document.xml" ||
				strings.HasPrefix(name, "word/header") ||
				strings.HasPrefix(name, "word/footer")
		}
	case strings.Contains(mimeType, "presentationml") || strings.HasSuffix(mimeType, "pptx"):
		keep = func(name string) bool {
			return strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml")
		}
	case strings.Contains(mimeType, "spreadsheetml") || strings.HasSuffix(mimeType, "xlsx"):
		keep = func(name string) bool {
			return name == "xl/sharedStrings.xml" ||
				strings.HasPrefix(name, "xl/worksheets/sheet")
		}
	default:
		// Try a generic walk — pull text from any .xml inside the zip
		keep = func(name string) bool { return strings.HasSuffix(name, ".xml") }
	}

	var out strings.Builder
	for _, f := range r.File {
		if !keep(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		text := stripXMLMarkup(raw)
		if text != "" {
			out.WriteString(text)
			out.WriteString("\n")
		}
		if out.Len() >= maxBytes {
			break
		}
	}

	result := strings.TrimSpace(out.String())
	if len(result) > maxBytes {
		result = result[:maxBytes] + "..."
	}
	return result, nil
}

// stripXMLMarkup walks an XML byte slice and returns just the character data,
// joined with spaces. We use the standard xml.Decoder rather than a regex
// because Office XML uses namespaces and entities that a regex would mangle.
func stripXMLMarkup(raw []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if cd, ok := tok.(xml.CharData); ok {
			s := strings.TrimSpace(string(cd))
			if s != "" {
				b.WriteString(s)
				b.WriteByte(' ')
			}
		}
	}
	// Collapse runs of whitespace
	return strings.Join(strings.Fields(b.String()), " ")
}

// IsOfficeDoc returns true for DOCX/PPTX/XLSX MIME types we know how to
// extract text from.
func IsOfficeDoc(mimeType string) bool {
	return strings.Contains(mimeType, "wordprocessingml") ||
		strings.Contains(mimeType, "presentationml") ||
		strings.Contains(mimeType, "spreadsheetml") ||
		strings.HasSuffix(mimeType, "docx") ||
		strings.HasSuffix(mimeType, "pptx") ||
		strings.HasSuffix(mimeType, "xlsx")
}

package api

import (
	"testing"
)

func TestIsDocumentImageAndTableFormatting(t *testing.T) {
	tests := []struct {
		fileName     string
		mimeType     string
		sampleText   string
		wantDoc      bool
		wantTableFmt bool
	}{
		{
			fileName:     "24 students Proctee list.PNG",
			mimeType:     "image/png",
			sampleText:   "Name of the Proctor: ANUSHA N\nSI NO Name USN\n1 AT Srikar Reddy 1TD25CS001",
			wantDoc:      true,
			wantTableFmt: true,
		},
		{
			fileName:     "IMG-20260910-WA0001.jpg",
			mimeType:     "image/jpeg",
			sampleText:   "TAX INVOICE Total: 450.00 GST: 81.00 Date: 12-09-2026",
			wantDoc:      true,
			wantTableFmt: true,
		},
		{
			fileName:     "sunset_beach.jpg",
			mimeType:     "image/jpeg",
			sampleText:   "",
			wantDoc:      false,
			wantTableFmt: false,
		},
		{
			fileName:     "family_selfie.png",
			mimeType:     "image/png",
			sampleText:   "happy birthday",
			wantDoc:      false,
			wantTableFmt: false,
		},
		{
			fileName:     "exam_timetable_v2.jpg",
			mimeType:     "image/jpeg",
			sampleText:   "",
			wantDoc:      true,
			wantTableFmt: false,
		},
		{
			fileName:     "train_ticket_pnr.png",
			mimeType:     "image/png",
			sampleText:   "PNR: 4656526133 Berth: 42 Seat no: 42",
			wantDoc:      true,
			wantTableFmt: true,
		},
	}

	for _, tc := range tests {
		gotDoc := IsDocumentImage(tc.fileName, tc.mimeType, tc.sampleText)
		if gotDoc != tc.wantDoc {
			t.Errorf("IsDocumentImage(%q, %q, %q) = %v, want %v", tc.fileName, tc.mimeType, tc.sampleText, gotDoc, tc.wantDoc)
		}
		gotFmt := needsTableFormatting(tc.sampleText)
		if gotFmt != tc.wantTableFmt {
			t.Errorf("needsTableFormatting(%q) = %v, want %v", tc.sampleText, gotFmt, tc.wantTableFmt)
		}
	}
}

func TestReadableForExtraction(t *testing.T) {
	// Document images should be readable
	if !ReadableForExtraction("image/png", "24 students Proctee list.PNG") {
		t.Errorf("expected proctee list image to be readable for extraction")
	}
	if !ReadableForExtraction("image/jpeg", "receipt_helmet.jpg") {
		t.Errorf("expected receipt image to be readable for extraction")
	}
	// Normal photos without document names should not be marked readable for extraction
	if ReadableForExtraction("image/jpeg", "vacation_sunset.jpg") {
		t.Errorf("expected vacation photo not to be readable for extraction")
	}
	// Audio/video should not be readable
	if ReadableForExtraction("audio/mpeg", "recording.mp3") {
		t.Errorf("expected audio not to be readable")
	}
	// Standard documents should be readable
	if !ReadableForExtraction("application/pdf", "syllabus.pdf") {
		t.Errorf("expected pdf to be readable")
	}
	if !ReadableForExtraction("application/vnd.openxmlformats-officedocument.wordprocessingml.document", "report.docx") {
		t.Errorf("expected docx to be readable")
	}
}

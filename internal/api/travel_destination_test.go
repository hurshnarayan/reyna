package api

import (
	"testing"
)

func TestIsFactualQuestion(t *testing.T) {
	questions := []string{
		"at what time is my departure to Hyderabad",
		"what is module 1 about",
		"which room is the exam in?",
		"when does the train arrive",
		"where is my car insurance?",
		"how much was the electricity bill",
		"kab hai meri train",
		"kya timing hai flight ki",
		"kitna bill aaya",
	}
	for _, q := range questions {
		if !isFactualQuestion(q) {
			t.Errorf("expected isFactualQuestion(%q) = true, got false", q)
		}
	}

	nonQuestions := []string{
		"train ticket",
		"notes for chemistry",
		"fetch me the 465 pdf",
		"electricity bill",
		"physics syllabus",
	}
	for _, q := range nonQuestions {
		if isFactualQuestion(q) {
			t.Errorf("expected isFactualQuestion(%q) = false, got true", q)
		}
	}
}

func TestFolderMatchesWhatGeneral(t *testing.T) {
	cases := []struct {
		folder string
		what   string
		want   bool
	}{
		{"Travel Tickets", "ticket", true},
		{"Invoices 2026", "invoice", true},
		{"Electricity", "bijli electricity bill", true},
		{"Operating Systems", "operating systems", true},
		{"Tax Documents", "income tax", true},
		{"Random Folder", "ticket", false},
	}
	for _, tc := range cases {
		got := folderMatchesWhat(tc.folder, tc.what)
		if got != tc.want {
			t.Errorf("folderMatchesWhat(%q, %q) = %v, want %v", tc.folder, tc.what, got, tc.want)
		}
	}
}

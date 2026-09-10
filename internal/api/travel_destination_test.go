package api

import (
	"testing"

	"github.com/hurshnarayan/reyna/internal/model"
	"github.com/hurshnarayan/reyna/internal/repository"
)

func TestPreferDestinationMatchesUsesRouteDirection(t *testing.T) {
	scored := []repository.ScoredFile{
		{File: model.File{ID: 2182, FileName: "2742758988.pdf"}, Score: 10, Coverage: 0.5},
		{File: model.File{ID: 2181, FileName: "4656526133.pdf"}, Score: 10, Coverage: 0.5},
		{File: model.File{ID: 1985, FileName: "old-ticket.pdf"}, Score: 10, Coverage: 0.5},
	}
	contents := map[int64]string{
		2182: "Booked from | To\nSECUNDERABAD JN (SC) | KSR BENGALURU (SBC)\nDeparture* 17:25",
		2181: "Booked from | To\nKSR BENGALURU (SBC) | KSR BENGALURU (SBC) | SECUNDERABAD JN (SC)\nDeparture* 20:00",
		1985: "From : BARAUNI JN (BJU) | To : SMVT BENGALURU (SMVB)",
	}

	got := preferDestinationMatches("at what time is my departure to Hyderabad", scored, contents)
	if len(got) != 1 || got[0].ID != 2181 {
		t.Fatalf("matches = %+v, want only outbound Hyderabad ticket 2181", got)
	}
}

func TestPreferDestinationMatchesLeavesUnclearQueryAlone(t *testing.T) {
	scored := []repository.ScoredFile{
		{File: model.File{ID: 1}},
		{File: model.File{ID: 2}},
	}
	got := preferDestinationMatches("what is module 1 about", scored, nil)
	if len(got) != 2 {
		t.Fatalf("matches = %d, want original ambiguous candidates", len(got))
	}
}

func TestDestinationSearchWhatAddsHyderabadStationAlias(t *testing.T) {
	got := destinationSearchWhat("departure to Hyderabad", "at what time is my departure to Hyderabad")
	if got != "departure to Hyderabad secunderabad" {
		t.Fatalf("search what = %q", got)
	}
}

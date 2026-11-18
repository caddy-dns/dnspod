package dnspod

import (
	"context"
	"testing"
	"time"

	"github.com/libdns/libdns"
)

func TestProvider(t *testing.T) {
	// Token from Caddyfile
	token := "123456,123456"
	zone := "example.org"
	recordName := "dnspodtest" // results in dnspodtest.example.org

	p := &Provider{
		APIToken: token,
	}

	ctx := context.Background()

	// 1. Cleanup
	recs, _ := p.GetRecords(ctx, zone)
	for _, r := range recs {
		if r.RR().Name == recordName {
			p.DeleteRecords(ctx, zone, []libdns.Record{r})
		}
	}

	// 2. Append
	t.Logf("Testing AppendRecords")
	records := []libdns.Record{
		Record{
			base: libdns.RR{
				Type: "TXT",
				Name: recordName,
				Data: "test-v1",
				TTL:  600 * time.Second,
			},
		},
	}
	appended, err := p.AppendRecords(ctx, zone, records)
	if err != nil {
		t.Fatalf("AppendRecords failed: %v", err)
	}
	recordID := appended[0].(Record).ID
	if rr := appended[0].RR(); rr.Type != "TXT" || rr.Data != "test-v1" {
		t.Fatalf("AppendRecords returned %+v, want the appended TXT record", rr)
	}

	// 3. SetRecords (Update via fallback)
	t.Logf("Testing SetRecords (Update)")
	updateRecord := Record{
		ID: recordID,
		base: libdns.RR{
			Type: "TXT",
			Name: recordName,
			Data: "test-v2",
			TTL:  600 * time.Second,
		},
	}

	updated, err := p.SetRecords(ctx, zone, []libdns.Record{updateRecord})
	if err != nil {
		t.Fatalf("SetRecords Update failed: %v", err)
	}

	// Since the response from API might be sparse, we verify by fetching again
	t.Logf("Verifying update via GetRecords")
	got, err := p.GetRecords(ctx, zone)
	if err != nil {
		t.Fatalf("GetRecords for verification failed: %v", err)
	}

	found := false
	for _, r := range got {
		if r.RR().Name == recordName && r.RR().Data == "test-v2" {
			found = true
			t.Logf("Found updated record: %+v", r)
			break
		}
	}
	if !found {
		t.Errorf("Updated record not found or value mismatch")
	} else {
		t.Logf("Update success!")
	}

	// 4. Cleanup
	p.DeleteRecords(ctx, zone, updated)
}

package receipt

import (
	"bytes"
	v "github.com/smeltery/bellows/internal/verification"
	"strings"
	"testing"
	"time"
)

func TestReuseProvenance(t *testing.T) {
	old := fixture(t)
	before, err := Encode(old)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(before)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := Encode(decoded)
	if !bytes.Equal(before, after) || decoded.Version != 1 {
		t.Fatal("v1 changed")
	}
	for _, kind := range []string{"valid", "v1", "failed", "duration", "same run", "future", "expired", "digest", "offset"} {
		t.Run(kind, func(t *testing.T) {
			r := fixture(t)
			r.Version = 2
			r.Results[0].DurationMS = 0
			p := &v.Reuse{ReceiptID: strings.Repeat("a", 64), RunID: "original", CompletedAt: r.StartedAt.Add(-time.Hour), ExpiresAt: r.StartedAt.Add(time.Hour)}
			r.Results[0].Reuse = p
			switch kind {
			case "v1":
				r.Version = 1
			case "failed":
				r.Results[0].State = v.Error
			case "duration":
				r.Results[0].DurationMS = 1
			case "same run":
				p.RunID = r.RunID
			case "future":
				p.CompletedAt = r.StartedAt.Add(time.Minute)
			case "expired":
				p.ExpiresAt = r.StartedAt
			case "digest":
				p.ReceiptID = "bad"
			case "offset":
				p.CompletedAt = p.CompletedAt.In(time.FixedZone("offset", 3600))
			}
			data, err := Encode(r)
			if kind != "valid" {
				if err == nil {
					t.Fatal("invalid provenance accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Decode(data); err != nil {
				t.Fatal(err)
			}
			for _, mutated := range []string{strings.Replace(string(data), `"reuse":{`, `"reuse":{"extra":1,`, 1), strings.Replace(string(data), `"reuse":{`, `"reuse":null,"unused":{`, 1), strings.Replace(string(data), p.CompletedAt.Format(time.RFC3339), p.CompletedAt.Format("2006-01-02T15:04:05+00:00"), 1)} {
				if _, err := Decode([]byte(mutated)); err == nil {
					t.Fatal("malformed provenance accepted")
				}
			}
		})
	}
}

package hme

import "testing"

func TestParseAliasListIncludesCreationTime(t *testing.T) {
	body := `{
		"aliases": [
			{
				"hme": "first@icloud.com",
				"anonymousId": "first",
				"state": "active",
				"createTimestamp": 1726531200000
			},
			{
				"hme": "second@icloud.com",
				"anonymousId": "second",
				"state": "active",
				"metaData": {"createdAt": "2026-09-17T00:00:00Z"}
			}
		]
	}`

	aliases := parseAliasList(body)
	if len(aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(aliases))
	}
	if aliases[0].CreatedAt != "1726531200000" {
		t.Fatalf("expected numeric creation time, got %q", aliases[0].CreatedAt)
	}
	if aliases[1].CreatedAt != "2026-09-17T00:00:00Z" {
		t.Fatalf("expected metadata creation time, got %q", aliases[1].CreatedAt)
	}
}

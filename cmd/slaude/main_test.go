package main

import "testing"

func TestParseThreadURL(t *testing.T) {
	tests := []struct {
		name                               string
		url                                string
		wantCh, wantTS, wantID, wantAfter string
	}{
		{
			name:   "basic URL without fragment",
			url:    "https://team.slack.com/archives/C123/p1700000001000000",
			wantCh: "C123", wantTS: "1700000001.000000",
		},
		{
			name:   "URL with instanceID",
			url:    "https://team.slack.com/archives/C123/p1700000001000000#dog",
			wantCh: "C123", wantTS: "1700000001.000000", wantID: "dog",
		},
		{
			name:    "URL with instanceID and afterTS",
			url:     "https://team.slack.com/archives/C123/p1700000001000000#dog@1700000005.000123",
			wantCh:  "C123", wantTS: "1700000001.000000",
			wantID:  "dog",
			wantAfter: "1700000005.000123",
		},
		{
			name:   "URL with instanceID, no afterTS (trailing @)",
			url:    "https://team.slack.com/archives/C123/p1700000001000000#dog@",
			wantCh: "C123", wantTS: "1700000001.000000",
			wantID: "dog", wantAfter: "",
		},
		{
			name:   "URL with query string",
			url:    "https://team.slack.com/archives/C123/p1700000001000000?foo=bar#fox_face",
			wantCh: "C123", wantTS: "1700000001.000000", wantID: "fox_face",
		},
		{
			name:    "URL with query string and afterTS",
			url:     "https://team.slack.com/archives/C123/p1700000001000000?foo=bar#fox_face@1700000099.000456",
			wantCh:  "C123", wantTS: "1700000001.000000",
			wantID:  "fox_face",
			wantAfter: "1700000099.000456",
		},
		{
			name: "invalid URL",
			url:  "https://example.com/foo",
		},
		{
			name:   "fragment only, no valid URL",
			url:    "not-a-url#dog@123.456",
			wantID: "dog", wantAfter: "123.456",
		},
		{
			name:   "real Slack URL with long channel ID",
			url:    "https://kubernetes.slack.com/archives/D1KFZ7GJ0/p1773044269918249#dog@1773044300.000100",
			wantCh: "D1KFZ7GJ0", wantTS: "1773044269.918249",
			wantID: "dog", wantAfter: "1773044300.000100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch, ts, id, after := parseThreadURL(tt.url)
			if ch != tt.wantCh {
				t.Errorf("channel: got %q, want %q", ch, tt.wantCh)
			}
			if ts != tt.wantTS {
				t.Errorf("threadTS: got %q, want %q", ts, tt.wantTS)
			}
			if id != tt.wantID {
				t.Errorf("instanceID: got %q, want %q", id, tt.wantID)
			}
			if after != tt.wantAfter {
				t.Errorf("afterTS: got %q, want %q", after, tt.wantAfter)
			}
		})
	}
}

package storage

import "testing"

func TestParseURI(t *testing.T) {
	bucket, key, err := ParseURI("s3://vod2live/hls/org/asset/master.m3u8")
	if err != nil || bucket != "vod2live" || key != "hls/org/asset/master.m3u8" {
		t.Fatalf("unexpected URI parse: %q %q %v", bucket, key, err)
	}
}

func TestParseURIRejectsNonStorageURL(t *testing.T) {
	if _, _, err := ParseURI("https://cdn.example.com/master.m3u8"); err == nil {
		t.Fatal("expected non-s3 URI to be rejected")
	}
}

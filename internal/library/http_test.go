package library

import "testing"

func TestAcceptedUploadType(t *testing.T) {
	for _, contentType := range []string{"video/mp4", "video/mp4; charset=binary", "video/webm", "application/octet-stream"} {
		if !acceptedUploadType(contentType) {
			t.Fatalf("expected content type %q to be accepted", contentType)
		}
	}
	for _, contentType := range []string{"text/plain", "application/pdf", "", "video/mpeg"} {
		if acceptedUploadType(contentType) {
			t.Fatalf("expected content type %q to be rejected", contentType)
		}
	}
}

func TestMaxUploadBytes(t *testing.T) {
	if MaxUploadBytes <= 0 || MaxUploadBytes > 50<<30 {
		t.Fatalf("unexpected upload limit: %d", MaxUploadBytes)
	}
}

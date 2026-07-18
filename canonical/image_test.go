package canonical

import "testing"

func TestImageModeConstants(t *testing.T) {
	if ImageModeGenerate != "generate" || ImageModeEdit != "edit" {
		t.Fatal()
	}
	if ImageFormatB64 != "b64" || ImageFormatURL != "url" {
		t.Fatal()
	}
}

func TestVideoStatusConstants(t *testing.T) {
	if VideoStatusProcessing != "processing" || VideoStatusCompleted != "completed" {
		t.Fatal()
	}
}

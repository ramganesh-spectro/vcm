package vsphere

import "testing"

func TestParseDatastorePathBracketForm(t *testing.T) {
	name, path, err := ParseDatastorePath("[datastore1] iso/installer.iso")
	if err != nil {
		t.Fatalf("ParseDatastorePath returned error: %v", err)
	}

	if name != "datastore1" {
		t.Fatalf("name = %q", name)
	}
	if path != "iso/installer.iso" {
		t.Fatalf("path = %q", path)
	}
}

func TestParseDatastorePathColonForm(t *testing.T) {
	name, path, err := ParseDatastorePath("datastore1:iso/installer.iso")
	if err != nil {
		t.Fatalf("ParseDatastorePath returned error: %v", err)
	}

	if name != "datastore1" {
		t.Fatalf("name = %q", name)
	}
	if path != "iso/installer.iso" {
		t.Fatalf("path = %q", path)
	}
}

func TestParseDatastorePathRejectsMissingRemotePath(t *testing.T) {
	if _, _, err := ParseDatastorePath("[datastore1]"); err == nil {
		t.Fatal("ParseDatastorePath returned nil error")
	}
}

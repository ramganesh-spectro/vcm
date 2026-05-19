package vsphere

import "testing"

func TestValidateCDAttachSpecRequiresCoreFields(t *testing.T) {
	tests := []CDAttachSpec{
		{},
		{VM: "ram-clone-1"},
		{VM: "ram-clone-1", ISOPath: "[vsanDatastore1] iso/seed.iso", Device: -1},
	}

	for _, test := range tests {
		if err := validateCDAttachSpec(test); err == nil {
			t.Fatalf("validateCDAttachSpec(%+v) returned nil error", test)
		}
	}
}

func TestValidateCDAttachSpecAcceptsCompleteSpec(t *testing.T) {
	spec := CDAttachSpec{
		VM:      "ram-clone-1",
		ISOPath: "[vsanDatastore1] iso/seed.iso",
		Device:  1,
	}

	if err := validateCDAttachSpec(spec); err != nil {
		t.Fatalf("validateCDAttachSpec returned error: %v", err)
	}
}

func TestValidateCloneSpecRequiresCoreFields(t *testing.T) {
	tests := []CloneSpec{
		{},
		{Source: "base-template"},
		{Source: "base-template", Name: "ram-test-01"},
		{Source: "base-template", Name: "ram-test-01", Folder: "sp-ramganesh.senthilkumar"},
	}

	for _, test := range tests {
		if err := validateCloneSpec(test); err == nil {
			t.Fatalf("validateCloneSpec(%+v) returned nil error", test)
		}
	}
}

func TestValidateCloneSpecAcceptsCompleteSpec(t *testing.T) {
	spec := CloneSpec{
		Source:    "base-template",
		Name:      "ram-test-01",
		Folder:    "sp-ramganesh.senthilkumar",
		Datastore: "vsanDatastore1",
	}

	if err := validateCloneSpec(spec); err != nil {
		t.Fatalf("validateCloneSpec returned error: %v", err)
	}
}

func TestVMListPatternRecursesWithEllipsis(t *testing.T) {
	if got := vmListPattern("sp-ramganesh.senthilkumar"); got != "sp-ramganesh.senthilkumar/..." {
		t.Fatalf("vmListPattern = %q, want sp-ramganesh.senthilkumar/...", got)
	}
	if got := vmListPattern("/Datacenter/vm/Lab"); got != "/Datacenter/vm/Lab/..." {
		t.Fatalf("vmListPattern = %q", got)
	}
	if got := vmListPattern("/Datacenter/vm/Lab/..."); got != "/Datacenter/vm/Lab/..." {
		t.Fatalf("vmListPattern = %q", got)
	}
	if got := vmListPattern("Lab/*"); got != "Lab/*" {
		t.Fatalf("vmListPattern = %q", got)
	}
}

func TestClonedVMPath(t *testing.T) {
	got := clonedVMPath("/Datacenter/vm/sp-ramganesh.senthilkumar/", "ram-test-01")
	want := "/Datacenter/vm/sp-ramganesh.senthilkumar/ram-test-01"
	if got != want {
		t.Fatalf("clonedVMPath = %q, want %q", got, want)
	}
}

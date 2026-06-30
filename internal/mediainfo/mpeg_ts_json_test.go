package mediainfo

import "testing"

func TestSetEAC3CommercialJSON_JOCGatesBDAVMetadata(t *testing.T) {
	info := ac3Info{hasJOC: true}

	tsExtras := map[string]string{}
	setEAC3CommercialJSON(tsExtras, info, false)
	if got := tsExtras["Format_Commercial_IfAny"]; got != "Dolby Digital Plus with Dolby Atmos" {
		t.Fatalf("non-BDAV commercial label = %q", got)
	}
	if _, ok := tsExtras["Format_Profile"]; ok {
		t.Fatal("non-BDAV JOC must not set Format_Profile")
	}
	if _, ok := tsExtras["MuxingMode"]; ok {
		t.Fatal("non-BDAV JOC must not set MuxingMode")
	}

	bdavExtras := map[string]string{}
	setEAC3CommercialJSON(bdavExtras, info, true)
	if got := bdavExtras["Format_Commercial_IfAny"]; got != "Dolby Digital Plus with Dolby Atmos" {
		t.Fatalf("BDAV commercial label = %q", got)
	}
	if got := bdavExtras["Format_Profile"]; got != "Blu-ray Disc" {
		t.Fatalf("BDAV Format_Profile = %q", got)
	}
	if got := bdavExtras["MuxingMode"]; got != "Stream extension" {
		t.Fatalf("BDAV MuxingMode = %q", got)
	}
}

func TestSetEAC3CommercialJSON_NonJOCUsesDolbyDigitalPlus(t *testing.T) {
	extras := map[string]string{}
	setEAC3CommercialJSON(extras, ac3Info{}, false)

	if got := extras["Format_Commercial_IfAny"]; got != "Dolby Digital Plus" {
		t.Fatalf("commercial label = %q", got)
	}
}

package kitty

import "testing"

func TestParseTransmitHeaderOrderAndDefaults(t *testing.T) {
	frame, failure := parseCompleteFrame([]byte("Gi=7,v=2,s=3,f=24,a=t;YWJj"), false)
	if failure != nil {
		t.Fatalf("failure=%#v", failure)
	}
	if frame.command.Action != ActionTransmit || frame.command.Image != 7 || frame.command.Decode.Format != FormatRGB24 || frame.command.Decode.Width != 3 || frame.command.Decode.Height != 2 || string(frame.payload) != "YWJj" {
		t.Fatalf("frame=%#v", frame)
	}
}

func TestParseRejectsDuplicatesUnknownExternalAndConflicts(t *testing.T) {
	cases := []string{
		"Ga=t,i=1,i=1,s=1,v=1;AAAA", "Ga=t,i=1,s=1,v=1,Z=2;AAAA",
		"Ga=t,i=1,t=f,s=1,v=1;AAAA", "Ga=t,i=1,f=100,s=1;AAAA",
		"Ga=t,i=1,f=100,o=z;AAAA", "Ga=p,i=1,p=1,c=2;",
		"Ga=d,i=9", "Ga=d,i=9,d=c",
	}
	for _, input := range cases {
		if _, failure := parseCompleteFrame([]byte(input), false); failure == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestParsePlacementAndDeleteNormalization(t *testing.T) {
	frame, failure := parseCompleteFrame([]byte("Ga=p,i=2,p=3,c=4,r=5,x=1,y=2,w=3,h=4,z=-2,C=1"), false)
	if failure != nil || frame.command.Placement == nil {
		t.Fatalf("frame=%#v failure=%#v", frame, failure)
	}
	p := frame.command.Placement
	if p.ID != 3 || p.Cols != 4 || p.Rows != 5 || p.Z != -2 || p.MoveCursor || p.Crop == nil || p.Crop.X != 1 || p.Crop.Height != 4 {
		t.Fatalf("placement=%#v", p)
	}
	frame, failure = parseCompleteFrame([]byte("Ga=d,i=9,d=I"), false)
	if failure != nil || frame.command.Delete == nil || frame.command.Delete.Image == nil || !frame.command.Delete.DeleteResource || !frame.command.Delete.WireIDsOnly {
		t.Fatalf("delete=%#v failure=%#v", frame, failure)
	}
	frame, failure = parseCompleteFrame([]byte("Ga=d,d=A"), false)
	if failure != nil || frame.command.Delete == nil || !frame.command.Delete.All || !frame.command.Delete.DeleteResource || !frame.command.Delete.WireIDsOnly {
		t.Fatalf("delete all=%#v failure=%#v", frame, failure)
	}
}

func TestContinuationFieldMatrix(t *testing.T) {
	if _, failure := parseCompleteFrame([]byte("Gm=1;QUJD"), true); failure != nil {
		t.Fatalf("valid continuation=%#v", failure)
	}
	for _, input := range []string{"Gm=1,a=t;QUJD", "Gq=0;", "Gm=1;A", "Gm=2;AAAA"} {
		if _, failure := parseCompleteFrame([]byte(input), true); failure == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestKittyWireIDsAreRestrictedToLowHalf(t *testing.T) {
	valid := []string{
		"Ga=t,i=2147483647,s=1,v=1;AAAA",
		"Ga=p,i=2147483647,p=2147483647,c=1,r=1;",
		"Ga=d,i=2147483647,d=I",
	}
	for _, input := range valid {
		if _, failure := parseCompleteFrame([]byte(input), false); failure != nil {
			t.Fatalf("rejected low-half boundary %q: %#v", input, failure)
		}
	}
	rejected := []string{
		"Ga=t,i=2147483648,s=1,v=1;AAAA",
		"Ga=p,i=1,p=2147483648,c=1,r=1;",
		"Ga=d,i=4294967295,d=I",
	}
	for _, input := range rejected {
		if _, failure := parseCompleteFrame([]byte(input), false); failure == nil || failure.code != ReplyInvalid {
			t.Fatalf("accepted internal namespace %q: %#v", input, failure)
		}
	}
}

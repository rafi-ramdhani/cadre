package conf

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	values, warnings := Parse(`# Permission mode for persona sessions
PERMISSION_MODE=default

ORCHESTRATOR_PERMISSION_MODE='auto'
RESUME="no"
EMPTY=
touch /tmp/marker
NAME=$(touch /tmp/marker)
A=b; touch /tmp/marker
B=x y
C="a$HOME"
export D=1
`)
	want := map[string]string{"PERMISSION_MODE": "default", "ORCHESTRATOR_PERMISSION_MODE": "auto", "RESUME": "no", "EMPTY": ""}
	if !reflect.DeepEqual(values, want) {
		t.Errorf("values %v, want %v", values, want)
	}
	if len(warnings) != 6 {
		t.Fatalf("warnings: %q", warnings)
	}
	for _, w := range []string{"line 7 ignored", "touch /tmp/marker", "line 8", "line 9", "line 10", "line 11", "line 12"} {
		if !strings.Contains(strings.Join(warnings, "\n"), w) {
			t.Errorf("no warning with %q in %q", w, warnings)
		}
	}
}

func TestTheTemplateReadsAsBefore(t *testing.T) {
	values, warnings := Parse("# Permission mode for persona sessions: default, acceptEdits, auto, ...\nPERMISSION_MODE=default\n")
	if values["PERMISSION_MODE"] != "default" || len(warnings) != 0 {
		t.Errorf("%v %q", values, warnings)
	}
}

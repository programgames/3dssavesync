package main

import (
	"strings"
	"testing"
)

func TestEnglish(t *testing.T) {
	english = true
	t.Cleanup(func() { english = false })
	reset()
	responses = []string{"Keep PC save"}
	srv := &Version{Hash: strings.Repeat("b", 64), Client: "3ds", Age: new(int64)}
	*srv.Age = 7200
	choice := resolveConflict(&Game{Name: "Pokemon X"}, &Local{Version: Version{Hash: strings.Repeat("a", 64)}}, srv, nil, false)
	if choice != keepPC {
		t.Errorf("« Keep PC save » : %v, attendu %v", choice, keepPC)
	}
	if len(questions) != 1 || !strings.Contains(questions[0], "Conflict on Pokemon X") || !strings.Contains(questions[0], "2 h ago") {
		t.Errorf("texte du Conflit en anglais attendu : %q", questions)
	}
	if !strings.HasPrefix(tr("Oui", "Yes"), "Yes") {
		t.Error("tr : anglais attendu")
	}
}

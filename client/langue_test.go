package main

import (
	"strings"
	"testing"
)

func TestAnglais(t *testing.T) {
	anglais = true
	t.Cleanup(func() { anglais = false })
	reinitialiser()
	reponses = []string{"Keep the PC"}
	srv := &Version{Hash: strings.Repeat("b", 64), Client: "3ds", Age: new(int64)}
	*srv.Age = 7200
	choix := resoudreConflit(&Jeu{Nom: "Pokemon X"}, &Local{Version: Version{Hash: strings.Repeat("a", 64)}}, srv, nil, false)
	if choix != forcer {
		t.Errorf("« Keep the PC » : %s, attendu %s", choix, forcer)
	}
	if len(questions) != 1 || !strings.Contains(questions[0], "Conflict: Pokemon X") || !strings.Contains(questions[0], "2 h ago") {
		t.Errorf("texte du Conflit en anglais attendu : %q", questions)
	}
	if !strings.HasPrefix(tr("Oui", "Yes"), "Yes") {
		t.Error("tr : anglais attendu")
	}
}

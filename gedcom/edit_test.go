package gedcom

import "testing"

func TestSetEventDate(t *testing.T) {
	g := chargerMini(t)
	jean, _ := g.Get("I0001")
	ancienne, err := jean.SetEventDate("BIRT", "5 JAN 1800")
	if err != nil {
		t.Fatal(err)
	}
	if ancienne != "1 JAN 1800" {
		t.Errorf("ancienne date = %q", ancienne)
	}
	if got := jean.Date("BIRT"); got != "5 JAN 1800" {
		t.Errorf("nouvelle date = %q", got)
	}

	// SetEventDate exige que le bloc "1 TAG" existe déjà (il n'insère qu'une DATE
	// dans un événement présent, il n'en crée pas) — comportement hérité de gedcom.py.
	if _, err := jean.SetEventDate("DEAT", "1 JAN 1880"); err == nil {
		t.Error("attendu une erreur : aucun bloc 1 DEAT sur I0001")
	}

	// Un événement présent mais sans DATE : SetEventDate doit l'insérer.
	sansDate := nouveauRecord([]string{"0 @I0099@ INDI", "1 DEAT", "1 CHAN"})
	if _, err := sansDate.SetEventDate("DEAT", "1 JAN 1880"); err != nil {
		t.Fatal(err)
	}
	if got := sansDate.Date("DEAT"); got != "1 JAN 1880" {
		t.Errorf("DATE insérée = %q", got)
	}
}

func TestAddCitation(t *testing.T) {
	g := chargerMini(t)
	jean, _ := g.Get("I0001")

	ajoute, err := jean.AddCitation("S0001", "")
	if err != nil || !ajoute {
		t.Fatalf("ajoute=%v err=%v", ajoute, err)
	}
	ajoute, err = jean.AddCitation("S0001", "")
	if err != nil || ajoute {
		t.Fatalf("citation dupliquée non détectée : ajoute=%v err=%v", ajoute, err)
	}

	ajoute, err = jean.AddCitation("S0001", "BIRT")
	if err != nil || !ajoute {
		t.Fatalf("citation sur événement : ajoute=%v err=%v", ajoute, err)
	}

	if _, err := jean.AddCitation("S0001", "DEAT"); err == nil {
		t.Error("attendu une erreur : DEAT n'existe pas sur I0001")
	}
}

func TestAddFamsAddFamc(t *testing.T) {
	g := chargerMini(t)
	paul, _ := g.Get("I0003")

	if got := paul.AddFams("F0009"); !got {
		t.Error("AddFams aurait dû ajouter F0009")
	}
	if got := paul.AddFams("F0009"); got {
		t.Error("AddFams dupliqué non détecté")
	}

	alice := nouveauRecord([]string{"0 @I0099@ INDI", "1 NAME Alice /Test/", "1 CHAN"})
	if got := alice.AddFamc("F0001"); !got {
		t.Error("AddFamc aurait dû ajouter F0001")
	}
	if got := alice.Valeur("FAMC"); got != "F0001" {
		t.Errorf("FAMC = %q", got)
	}
}

func TestTouchChan(t *testing.T) {
	g := chargerMini(t)
	paul, _ := g.Get("I0003") // porte déjà un bloc 1 CHAN dans la fixture
	paul.TouchChan("17 AUG 2026", "12:00:00")
	found := false
	for i, l := range paul.Lignes {
		if l == "1 CHAN" {
			found = true
			if paul.Lignes[i+1] != "2 DATE 17 AUG 2026" {
				t.Errorf("DATE = %q", paul.Lignes[i+1])
			}
			if paul.Lignes[i+2] != "3 TIME 12:00:00" {
				t.Errorf("TIME = %q", paul.Lignes[i+2])
			}
		}
	}
	if !found {
		t.Error("bloc 1 CHAN introuvable")
	}

	// Individu sans CHAN préexistant : doit en créer un.
	alice := nouveauRecord([]string{"0 @I0099@ INDI", "1 NAME Alice /Test/"})
	alice.TouchChan("17 AUG 2026", "12:00:00")
	if alice.Lignes[len(alice.Lignes)-3] != "1 CHAN" {
		t.Errorf("CHAN non ajouté : %v", alice.Lignes)
	}
}

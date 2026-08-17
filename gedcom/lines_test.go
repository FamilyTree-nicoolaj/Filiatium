package gedcom

import (
	"strings"
	"testing"
)

func TestDecoupe(t *testing.T) {
	cas := []struct {
		ligne string
		veut  Ligne
		ok    bool
	}{
		{"2 DATE 5 JUN 1674", Ligne{2, "", "DATE", "5 JUN 1674"}, true},
		{"0 @I0001@ INDI", Ligne{0, "I0001", "INDI", ""}, true},
		{"1 NAME Jean /Untel/", Ligne{1, "", "NAME", "Jean /Untel/"}, true},
		{"1 ADDR", Ligne{1, "", "ADDR", ""}, true}, // valeur vide, comme "0 @SUBM@ SUBM / 1 ADDR"
		{"pas une ligne gedcom", Ligne{}, false},
		{"", Ligne{}, false},
	}
	for _, c := range cas {
		got, ok := Decoupe(c.ligne)
		if ok != c.ok || got != c.veut {
			t.Errorf("Decoupe(%q) = %+v, %v — voulu %+v, %v", c.ligne, got, ok, c.veut, c.ok)
		}
	}
}

func TestEnligneCourte(t *testing.T) {
	got := Enligne(1, "TITL", "Acte de mariage")
	veut := []string{"1 TITL Acte de mariage"}
	if len(got) != 1 || got[0] != veut[0] {
		t.Errorf("Enligne courte = %v, voulu %v", got, veut)
	}
}

func TestEnligneRepliCONC(t *testing.T) {
	// Une valeur de 300 caractères ASCII doit se replier en CONC : "1 TAG " + 248
	// premiers caractères, puis le reste en "2 CONC ...". Compte des runes, pas des
	// octets — voir le piège documenté dans le plan (family.ged avait 186 lignes
	// > 255 octets mais 0 ligne > 255 caractères).
	valeur := strings.Repeat("a", 300)
	got := Enligne(1, "NOTE", valeur)
	if len(got) != 2 {
		t.Fatalf("attendu 2 lignes, obtenu %d : %v", len(got), got)
	}
	if got[0] != "1 NOTE "+strings.Repeat("a", limiteConc) {
		t.Errorf("première ligne inattendue : %q", got[0])
	}
	if got[1] != "2 CONC "+strings.Repeat("a", 300-limiteConc) {
		t.Errorf("ligne CONC inattendue : %q", got[1])
	}
}

func TestEnligneRunesPasOctets(t *testing.T) {
	// Une valeur avec des accents (2 octets/caractère en UTF-8) ne doit pas se
	// replier tant qu'elle ne dépasse pas 255 *runes*, même si elle dépasse 255 octets.
	valeur := strings.Repeat("é", 200) // 200 runes, 400 octets — sous la barre en runes
	got := Enligne(1, "NOTE", valeur)
	if len(got) != 1 {
		t.Errorf("valeur de 200 runes accentuées repliée à tort : %d lignes", len(got))
	}
}

func TestEnligneNoteMultiParagraphe(t *testing.T) {
	got := EnligneNote(1, "premier paragraphe\nsecond paragraphe")
	veut := []string{"1 NOTE premier paragraphe", "1 CONT second paragraphe"}
	if len(got) != 2 || got[0] != veut[0] || got[1] != veut[1] {
		t.Errorf("EnligneNote = %v, voulu %v", got, veut)
	}
}

func TestAnnee(t *testing.T) {
	cas := []struct {
		date string
		veut int
		ok   bool
	}{
		{"BET 1700 AND 1710", 1700, true},
		{"ABT 1676", 1676, true},
		{"17 DEC 1710", 1710, true},
		{"", 0, false},
		{"sans date", 0, false},
	}
	for _, c := range cas {
		got, ok := Annee(c.date)
		if ok != c.ok || got != c.veut {
			t.Errorf("Annee(%q) = %d, %v — voulu %d, %v", c.date, got, ok, c.veut, c.ok)
		}
	}
}

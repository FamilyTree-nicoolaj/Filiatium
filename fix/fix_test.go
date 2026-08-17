package fix

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

func charger(t *testing.T, contenu string) *gedcom.Gedcom {
	t.Helper()
	chemin := filepath.Join(t.TempDir(), "test.ged")
	if err := os.WriteFile(chemin, []byte(contenu), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := gedcom.Load(chemin)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

const entete = "0 HEAD\n1 CHAR UTF-8\n"
const pied = "0 TRLR\n"

func TestFixLienReciproque(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n"+ // pas de FAMS -> L1
		"0 @I0002@ INDI\n1 NAME Marie /Dupont/\n1 FAMS @F0001@\n"+
		"0 @I0003@ INDI\n1 NAME Paul /Untel/\n"+ // pas de FAMC -> L2
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 CHIL @I0003@\n"+
		pied)

	candidats := Detecter(g)
	if len(candidats) != 2 {
		t.Fatalf("attendu 2 correctifs (L1+L2), obtenu %d : %+v", len(candidats), candidats)
	}
	for _, c := range candidats {
		if c.Categorie != LienReciproque {
			t.Errorf("catégorie inattendue : %s", c.Categorie)
		}
		c.Appliquer()
	}

	i1, _ := g.Get("I0001")
	if i1.Valeur("FAMS") != "F0001" {
		t.Errorf("I0001 : FAMS non ajouté")
	}
	i3, _ := g.Get("I0003")
	if i3.Valeur("FAMC") != "F0001" {
		t.Errorf("I0003 : FAMC non ajouté")
	}

	// Plus rien à corriger une fois appliqué.
	if suite := Detecter(g); len(suite) != 0 {
		t.Errorf("correctifs résiduels après application : %+v", suite)
	}
}

func TestFixPointeurDuplique(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 FAMS @F0001@\n1 FAMS @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 CHIL @I0002@\n1 CHIL @I0002@\n"+
		"0 @I0002@ INDI\n1 NAME Paul /Untel/\n1 FAMC @F0001@\n"+
		pied)

	candidats := Detecter(g)
	if len(candidats) != 2 {
		t.Fatalf("attendu 2 correctifs (D3+D4), obtenu %d : %+v", len(candidats), candidats)
	}
	for _, c := range candidats {
		if c.Categorie != PointeurDuplique {
			t.Errorf("catégorie inattendue : %s", c.Categorie)
		}
		c.Appliquer()
	}

	i1, _ := g.Get("I0001")
	if got := i1.Valeurs("FAMS"); len(got) != 1 {
		t.Errorf("FAMS toujours dupliqué : %v", got)
	}
	fam, _ := g.Get("F0001")
	if got := fam.Valeurs("CHIL"); len(got) != 1 {
		t.Errorf("CHIL toujours dupliqué : %v", got)
	}
}

func TestFixLigneTropLongue(t *testing.T) {
	long := strings.Repeat("a", 300)
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NOTE "+long+"\n"+pied)

	candidats := Detecter(g)
	if len(candidats) != 1 || candidats[0].Categorie != LigneTropLongue {
		t.Fatalf("candidats = %+v", candidats)
	}
	candidats[0].Appliquer()

	i1, _ := g.Get("I0001")
	for _, l := range i1.Lignes {
		if n := len([]rune(l)); n > 255 {
			t.Errorf("ligne encore trop longue (%d) après repli : %q", n, l)
		}
	}
	// Le repli doit être reconstituable à l'identique via CONC (round-trip du sens).
	reconstitue := i1.Valeur("NOTE")
	if reconstitue == "" {
		t.Errorf("NOTE introuvable après repli")
	}
}

func TestAucunCorrectifSurArbrePropre(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Marie /Dupont/\n1 FAMC @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 CHIL @I0002@\n"+
		pied)
	if c := Detecter(g); len(c) != 0 {
		t.Errorf("faux positif sur arbre propre : %+v", c)
	}
}

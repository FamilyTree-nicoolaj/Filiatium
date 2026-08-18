package renumber

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/rules"
)

const entete = "0 HEAD\n1 CHAR UTF-8\n"
const pied = "0 TRLR\n"

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

func TestCalculerBFSDepuisSource(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 FAMS @F0001@\n1 FAMC @F0002@\n1 SOUR @S0001@\n"+
		"0 @I0002@ INDI\n1 NAME Marie /Autre/\n1 FAMS @F0001@\n"+
		"0 @I0003@ INDI\n1 NAME Enfant1 /Untel/\n1 FAMC @F0001@\n"+
		"0 @I0004@ INDI\n1 NAME Enfant2 /Untel/\n1 FAMC @F0001@\n"+
		"0 @I0005@ INDI\n1 NAME Pere /Untel/\n1 FAMS @F0002@\n"+
		"0 @I0006@ INDI\n1 NAME Mere /Autre/\n1 FAMS @F0002@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 CHIL @I0003@\n1 CHIL @I0004@\n"+
		"0 @F0002@ FAM\n1 HUSB @I0005@\n1 WIFE @I0006@\n1 CHIL @I0001@\n"+
		"0 @S0001@ SOUR\n1 TITL Un acte\n"+
		pied)

	table, err := Calculer(g, "I0001")
	if err != nil {
		t.Fatal(err)
	}

	// L'individu source est toujours numéroté en premier (I0001), et sa FAMC
	// (ascendance) est visitée avant sa FAMS (descendance) — ordre de parcours
	// fixé par le code, indépendant de l'ordre des lignes dans le fichier.
	voulu := map[string]string{
		"I0001": "I0001", // source, toujours en premier
		"I0005": "I0002", // père, atteint via FAMC (avant FAMS)
		"I0006": "I0003", // mère, atteint via FAMC
		"I0002": "I0004", // conjoint, atteint via FAMS
		"I0003": "I0005", // enfant
		"I0004": "I0006", // enfant
		"F0002": "F0001", // FAMC (ascendance), numérotée avant F0001
		"F0001": "F0002", // FAMS (descendance)
	}
	if !reflect.DeepEqual(table, voulu) {
		t.Errorf("table = %#v, voulu %#v", table, voulu)
	}
	if _, vu := table["S0001"]; vu {
		t.Error("S0001 (SOUR) ne doit jamais être renuméroté")
	}
}

func TestCalculerBalayeComposantesDeconnectees(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Isolee /Untel/\n"+ // source, sans aucune famille
		"0 @I0100@ INDI\n1 NAME Autre1 /Ailleurs/\n1 FAMS @F0050@\n"+
		"0 @I0101@ INDI\n1 NAME Autre2 /Ailleurs/\n1 FAMS @F0050@\n"+
		"0 @F0050@ FAM\n1 HUSB @I0100@\n1 WIFE @I0101@\n"+
		pied)

	table, err := Calculer(g, "I0001")
	if err != nil {
		t.Fatal(err)
	}
	voulu := map[string]string{
		"I0001": "I0001", // source
		"I0100": "I0002", // composante déconnectée, balayée en ordre fichier
		"I0101": "I0003",
		"F0050": "F0001",
	}
	if !reflect.DeepEqual(table, voulu) {
		t.Errorf("table = %#v, voulu %#v", table, voulu)
	}
}

// snapshot capture les lignes de tous les enregistrements, pour vérifier qu'un
// appel n'a rien modifié en place.
func snapshot(g *gedcom.Gedcom) []string {
	var out []string
	for _, r := range g.Records {
		out = append(out, r.Lignes...)
	}
	return out
}

func TestCalculerDeterministe(t *testing.T) {
	contenu := entete +
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 FAMS @F0001@\n" +
		"0 @I0002@ INDI\n1 NAME Marie /Autre/\n1 FAMS @F0001@\n" +
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n" +
		pied

	g1, g2 := charger(t, contenu), charger(t, contenu)
	avant := snapshot(g1)

	table1, err := Calculer(g1, "I0001")
	if err != nil {
		t.Fatal(err)
	}
	table2, err := Calculer(g2, "I0001")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(table1, table2) {
		t.Errorf("Calculer non déterministe : %#v vs %#v", table1, table2)
	}

	apres := snapshot(g1)
	if !reflect.DeepEqual(avant, apres) {
		t.Error("Calculer a modifié g — devrait être pure")
	}
}

func TestCalculerDanglingPointeurInchange(t *testing.T) {
	g := charger(t, entete+
		"0 @I0050@ INDI\n1 NAME Jean /Untel/\n1 FAMC @F9999@\n"+ // F9999 pendant
		"0 @I0051@ INDI\n1 NAME Marie /Autre/\n"+
		pied)

	avant := rules.S4(g, config.Seuils{})
	if len(avant) != 1 || avant[0].Message != "pointeur non résolu : @F9999@" {
		t.Fatalf("S4 avant = %+v", avant)
	}

	table, err := Calculer(g, "I0050")
	if err != nil {
		t.Fatal(err)
	}
	g.Renumeroter(table)

	apres := rules.S4(g, config.Seuils{})
	if len(apres) != 1 || apres[0].Message != avant[0].Message {
		t.Errorf("S4 après renumérotation = %+v, voulu identique à avant (%+v)", apres, avant)
	}
}

func TestAppliquerNotesRemplaceTokens(t *testing.T) {
	correspondance := map[string]string{"I0517": "I0000", "F0271": "F9999"}
	contenu := "Voir I0517 et [I0517] pour confirmation. **F0271** est cité ici : `1 FAMC @F0271@`.\n" +
		"Hors sujet : I9999 et I05170 ne doivent pas changer."

	nouveau, n := AppliquerNotes(contenu, correspondance)
	if n != 4 {
		t.Errorf("n = %d, voulu 4", n)
	}
	voulu := "Voir I0000 et [I0000] pour confirmation. **F9999** est cité ici : `1 FAMC @F9999@`.\n" +
		"Hors sujet : I9999 et I05170 ne doivent pas changer."
	if nouveau != voulu {
		t.Errorf("nouveau =\n%q\nvoulu\n%q", nouveau, voulu)
	}
}

func TestCalculerDecalage(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME A /A/\n"+
		"0 @F0002@ FAM\n"+
		"0 @I9999@ INDI\n1 NAME B /B/\n"+
		pied)

	table, err := CalculerDecalage(g, 5000)
	if err != nil {
		t.Fatal(err)
	}
	voulu := map[string]string{"I0001": "I5001", "F0002": "F5002", "I9999": "I14999"}
	if !reflect.DeepEqual(table, voulu) {
		t.Errorf("table = %#v, voulu %#v", table, voulu)
	}
}

func TestCalculerDecalageNegatifRefuse(t *testing.T) {
	g := charger(t, entete+"0 @I0001@ INDI\n1 NAME A /A/\n"+pied)
	if _, err := CalculerDecalage(g, -10000); err == nil {
		t.Error("décalage rendant le numéro négatif : voulu une erreur")
	}
}

func TestCalculerDecalageXrefSansChiffres(t *testing.T) {
	g := charger(t, entete+"0 @ABC@ INDI\n1 NAME A /A/\n"+pied)
	if _, err := CalculerDecalage(g, 5); err == nil {
		t.Error("xref sans suffixe numérique : voulu une erreur")
	}
}

func TestCalculerPrefixe(t *testing.T) {
	g := charger(t, entete+"0 @I0001@ INDI\n1 NAME A /A/\n0 @F0002@ FAM\n"+pied)
	table, err := CalculerPrefixe(g, "Z")
	if err != nil {
		t.Fatal(err)
	}
	voulu := map[string]string{"I0001": "ZI0001", "F0002": "ZF0002"}
	if !reflect.DeepEqual(table, voulu) {
		t.Errorf("table = %#v, voulu %#v", table, voulu)
	}
}

func TestCalculerPrefixeVideRefuse(t *testing.T) {
	g := charger(t, entete+"0 @I0001@ INDI\n1 NAME A /A/\n"+pied)
	if _, err := CalculerPrefixe(g, ""); err == nil {
		t.Error("--prefixe vide : voulu une erreur")
	}
}

// TestCalculerPrefixeCollisionEnregistrementExistant vérifie que validerTable
// intercepte, via le chemin normal de CalculerPrefixe, une collision entre un
// nouveau xref et un enregistrement non renuméroté (ici un SOUR gardé tel quel).
func TestCalculerPrefixeCollisionEnregistrementExistant(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME A /A/\n"+
		"0 @ZI0001@ SOUR\n1 TITL Coïncidence\n"+
		pied)
	if _, err := CalculerPrefixe(g, "Z"); err == nil {
		t.Error("collision avec un SOUR existant : voulu une erreur")
	}
}

func TestValiderTableCollisionBijection(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME A /A/\n0 @I0002@ INDI\n1 NAME B /B/\n"+pied)
	table := map[string]string{"I0001": "IX0001", "I0002": "IX0001"}
	if err := validerTable(g, table); err == nil {
		t.Error("deux anciens xref vers le même nouveau : voulu une erreur")
	}
}

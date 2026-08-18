package publish

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
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

// candidatsFixture mêle une impossibilité stricte (R10 : mariage après décès) et une
// règle à seuil (R7 : mère trop ÂGÉE — au-delà d'AgeMaxMere, jamais couvert par
// AgeMinParent/R4 qui ne vérifie qu'un minimum), pour vérifier que --niveau filtre
// bien l'une sans l'autre sans faire intervenir une troisième règle par accident.
const candidatsFixture = "" +
	"0 @I0001@ INDI\n1 NAME Epoux /Untel/\n1 DEAT\n2 DATE 1700\n1 FAMS @F0001@\n" +
	"0 @F0001@ FAM\n1 HUSB @I0001@\n1 MARR\n2 DATE 1710\n" +
	"0 @I0002@ INDI\n1 NAME Mere /Agee/\n1 BIRT\n2 DATE 1900\n1 FAMS @F0002@\n" +
	"0 @I0003@ INDI\n1 NAME Enfant /Agee/\n1 BIRT\n2 DATE 1955\n1 FAMC @F0002@\n" +
	"0 @F0002@ FAM\n1 WIFE @I0002@\n1 CHIL @I0003@\n"

func TestCalculerNiveauStrictNIgnoreLesReglesASeuil(t *testing.T) {
	g := charger(t, entete+candidatsFixture+pied)
	candidats := Calculer(g, NiveauStrict, config.Defauts())

	voulu := []Candidat{
		{Xref: "F0001", Tag: "MARR", Regle: "R10"},
		{Xref: "I0001", Tag: "DEAT", Regle: "R10"},
	}
	if len(candidats) != len(voulu) {
		t.Fatalf("candidats = %+v, voulu %d entrées", candidats, len(voulu))
	}
	for i, v := range voulu {
		if candidats[i].Xref != v.Xref || candidats[i].Tag != v.Tag || candidats[i].Regle != v.Regle {
			t.Errorf("candidats[%d] = %+v, voulu %+v", i, candidats[i], v)
		}
	}

	// Calculer ne modifie jamais g, et est pur : même entrée, même résultat.
	rejoue := Calculer(g, NiveauStrict, config.Defauts())
	if !reflect.DeepEqual(candidats, rejoue) {
		t.Errorf("Calculer non déterministe : %+v vs %+v", candidats, rejoue)
	}
}

func TestCalculerNiveauLargeAjouteLesReglesASeuil(t *testing.T) {
	g := charger(t, entete+candidatsFixture+pied)
	candidats := Calculer(g, NiveauLarge, config.Defauts())

	var trouveR7Mere, trouveR7Enfant bool
	for _, c := range candidats {
		if c.Xref == "I0002" && c.Tag == "BIRT" && c.Regle == "R7" {
			trouveR7Mere = true
		}
		if c.Xref == "I0003" && c.Tag == "BIRT" && c.Regle == "R7" {
			trouveR7Enfant = true
		}
	}
	if !trouveR7Mere || !trouveR7Enfant {
		t.Errorf("R7 (mère trop jeune) absent au niveau large : %+v", candidats)
	}
	if len(candidats) != 4 { // F0001.MARR, I0001.DEAT (R10) + I0002.BIRT, I0003.BIRT (R7)
		t.Errorf("candidats = %+v, voulu 4 entrées", candidats)
	}
}

func TestCalculerNiveauModereAjouteCoincidencesSuspectes(t *testing.T) {
	// Mariage recopié des parents (R1, sans seuil) : absent du niveau strict, présent
	// dès le niveau modéré.
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Père /Castel/\n1 FAMS @F0000@\n"+
		"0 @I0002@ INDI\n1 NAME Mère /Untel/\n1 FAMS @F0000@\n"+
		"0 @I0003@ INDI\n1 NAME Enfant /Castel/\n1 FAMC @F0000@\n1 FAMS @F0001@\n"+
		"0 @F0000@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 CHIL @I0003@\n1 MARR\n2 DATE 1 JAN 1700\n"+
		"0 @F0001@ FAM\n1 HUSB @I0003@\n1 MARR\n2 DATE 1 JAN 1700\n"+
		pied)

	if c := Calculer(g, NiveauStrict, config.Defauts()); len(c) != 0 {
		t.Errorf("niveau strict : R1 ne devrait pas apparaître : %+v", c)
	}
	c := Calculer(g, NiveauModere, config.Defauts())
	if len(c) != 1 || c[0].Xref != "F0001" || c[0].Tag != "MARR" || c[0].Regle != "R1" {
		t.Errorf("niveau modéré : R1 attendu sur F0001.MARR, obtenu %+v", c)
	}
}

func TestCalculerProtegeLesFaitsSourcesParEnregistrement(t *testing.T) {
	// I0001 porte une source (n'importe où sur la fiche) : son DEAT est protégé même
	// si F0001 (sans source) ne l'est pas — la protection est par ENREGISTREMENT.
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Epoux /Untel/\n1 DEAT\n2 DATE 1700\n1 SOUR @S0001@\n1 FAMS @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 MARR\n2 DATE 1710\n"+
		"0 @S0001@ SOUR\n1 TITL Acte\n"+
		pied)

	candidats := Calculer(g, NiveauStrict, config.Defauts())
	var epoux, fam *Candidat
	for i := range candidats {
		switch candidats[i].Xref {
		case "I0001":
			epoux = &candidats[i]
		case "F0001":
			fam = &candidats[i]
		}
	}
	if epoux == nil || !epoux.Sourced {
		t.Errorf("I0001.DEAT devrait être Sourced=true : %+v", epoux)
	}
	if fam == nil || fam.Sourced {
		t.Errorf("F0001.MARR devrait être Sourced=false : %+v", fam)
	}

	n := Appliquer(g, candidats)
	if n != 1 {
		t.Errorf("Appliquer = %d suppression(s), voulu 1", n)
	}
	if rec, _ := g.Get("F0001"); rec.Evenement("MARR") != nil {
		t.Error("F0001.MARR (non sourcé) aurait dû être supprimé")
	}
	if rec, _ := g.Get("I0001"); rec.Evenement("DEAT") == nil {
		t.Error("I0001.DEAT (sourcé) n'aurait jamais dû être supprimé")
	}
}

// TestCalculerDedupliqueEntreRegles vérifie qu'un même fait mis en cause par deux
// règles distinctes (ici R7 et R8, sur la naissance d'un enfant à double filiation)
// n'apparaît qu'une seule fois.
func TestCalculerDedupliqueEntreRegles(t *testing.T) {
	g := charger(t, entete+
		"0 @I0002@ INDI\n1 NAME Mere /Jeune/\n1 BIRT\n2 DATE 1900\n1 FAMS @F0001@\n"+
		"0 @I0003@ INDI\n1 NAME Enfant /Untel/\n1 BIRT\n2 DATE 1905\n1 FAMC @F0001@\n1 FAMC @F0002@\n"+
		"0 @F0001@ FAM\n1 WIFE @I0002@\n1 CHIL @I0003@\n"+ // mère à 5 ans -> R7
		"0 @I0004@ INDI\n1 NAME Pere /Age/\n1 BIRT\n2 DATE 1800\n1 FAMS @F0002@\n"+
		"0 @F0002@ FAM\n1 HUSB @I0004@\n1 CHIL @I0003@\n"+ // père à 105 ans -> R8
		pied)

	candidats := Calculer(g, NiveauLarge, config.Defauts())
	n := 0
	for _, c := range candidats {
		if c.Xref == "I0003" && c.Tag == "BIRT" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("I0003.BIRT (visé par R7 ET R8) apparaît %d fois, voulu 1 : %+v", n, candidats)
	}
	if len(candidats) != 3 { // I0002.BIRT, I0003.BIRT (dédupliqué), I0004.BIRT
		t.Errorf("candidats = %+v, voulu 3 entrées distinctes", candidats)
	}
}

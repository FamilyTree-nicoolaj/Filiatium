package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

// charger construit un Gedcom en mémoire à partir d'un contenu GEDCOM synthétique
// (personnages fictifs) — la vraie preuve de non-régression du portage est la
// parité de sortie avec les scripts Python sur le corpus réel (scripts/parite.sh) ;
// ces tests couvrent les cas représentatifs indépendamment de ce corpus.
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

func idsDe(findings []Finding) map[string]int {
	m := map[string]int{}
	for _, f := range findings {
		m[f.Regle]++
	}
	return m
}

const enteteMinimale = "0 HEAD\n1 CHAR UTF-8\n"
const piedMinimal = "0 TRLR\n"

func TestS3LigneTropLongue(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NOTE "+repete("x", 300)+"\n"+piedMinimal)
	if s3 := S3(g, Seuils{}); len(s3) != 1 {
		t.Errorf("S3 : attendu 1 signalement, obtenu %d : %v", len(s3), s3)
	}
}

func TestS2SautDeNiveau(t *testing.T) {
	// "2 GIVN" directement après "0 @I0001@ INDI" (niveau 0) : saute le niveau 1.
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n2 GIVN Jean\n"+piedMinimal)
	if s2 := S2(g, Seuils{}); len(s2) != 1 {
		t.Errorf("S2 : attendu 1 signalement, obtenu %d : %v", len(s2), s2)
	}
}

func TestS5FichierValide(t *testing.T) {
	g := charger(t, enteteMinimale+"0 @I0001@ INDI\n1 NAME Jean /Untel/\n"+piedMinimal)
	if s5 := S5(g, Seuils{}); len(s5) != 0 {
		t.Errorf("S5 : fichier HEAD...TRLR valide, attendu 0, obtenu %d : %v", len(s5), s5)
	}
}

func repete(s string, n int) string {
	out := make([]byte, 0, n)
	for len(out) < n {
		out = append(out, s...)
	}
	return string(out[:n])
}

func TestS5FichierIncomplet(t *testing.T) {
	g := charger(t, "0 @I0001@ INDI\n1 NAME Sans /Entete/\n")
	s5 := S5(g, Seuils{})
	if len(s5) != 2 { // ni HEAD, ni TRLR
		t.Errorf("S5 : attendu 2 signalements, obtenu %d : %v", len(s5), s5)
	}
}

func TestS4PointeurNonResolu(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 SOUR @S9999@\n"+piedMinimal)
	s4 := S4(g, Seuils{})
	if len(s4) != 1 || s4[0].Xrefs[0] != "S9999" {
		t.Errorf("S4 = %+v", s4)
	}
}

// TestReciprocite construit une FAM où HUSB pointe vers I0001 sans que I0001 porte
// FAMS en retour (le bug I0614/F0259 documenté dans controle_liens.py), et où le
// CHIL est bien réciproque — pour vérifier que L1 se déclenche et L2 non.
func TestReciprocite(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n"+ // pas de FAMS -> L1
		"0 @I0002@ INDI\n1 NAME Marie /Dupont/\n1 FAMS @F0001@\n"+
		"0 @I0003@ INDI\n1 NAME Paul /Untel/\n1 FAMC @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 CHIL @I0003@\n"+
		piedMinimal)

	l1 := L1(g, Seuils{})
	if len(l1) != 1 || l1[0].Xrefs[1] != "I0001" {
		t.Errorf("L1 = %+v", l1)
	}
	if l2 := L2(g, Seuils{}); len(l2) != 0 {
		t.Errorf("L2 : attendu 0 (CHIL réciproque), obtenu %+v", l2)
	}
}

func TestFamilleFantome(t *testing.T) {
	g := charger(t, enteteMinimale+"0 @F0001@ FAM\n"+piedMinimal)
	if l6 := L6(g, Seuils{}); len(l6) != 1 {
		t.Errorf("L6 = %+v", l6)
	}
}

// TestDoublonFamille reproduit le cas F0222/F0223 : deux FAM avec un conjoint commun
// et un enfant commun.
func TestDoublonFamille(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 FAMS @F0001@\n1 FAMS @F0002@\n"+
		"0 @I0002@ INDI\n1 NAME Marie /Dupont/\n1 FAMS @F0001@\n"+
		"0 @I0003@ INDI\n1 NAME Paul /Untel/\n1 FAMC @F0001@\n1 FAMC @F0002@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 CHIL @I0003@\n"+
		"0 @F0002@ FAM\n1 HUSB @I0001@\n1 CHIL @I0003@\n"+
		piedMinimal)
	d1 := D1(g, Seuils{})
	if len(d1) != 1 || idsDe(d1)["D1"] != 1 {
		t.Errorf("D1 = %+v", d1)
	}
}

func TestGermainsMaries(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Frère /Untel/\n1 FAMC @F0000@\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Soeur /Untel/\n1 FAMC @F0000@\n1 FAMS @F0001@\n"+
		"0 @F0000@ FAM\n1 CHIL @I0001@\n1 CHIL @I0002@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n"+
		piedMinimal)
	d2 := D2(g, Seuils{})
	if len(d2) != 1 {
		t.Errorf("D2 = %+v", d2)
	}
}

func TestPointeurRepete(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 FAMS @F0001@\n1 FAMS @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 CHIL @I0002@\n1 CHIL @I0002@\n"+
		"0 @I0002@ INDI\n1 NAME Paul /Untel/\n1 FAMC @F0001@\n"+
		piedMinimal)
	if d3 := D3(g, Seuils{}); len(d3) != 1 {
		t.Errorf("D3 = %+v", d3)
	}
	if d4 := D4(g, Seuils{}); len(d4) != 1 {
		t.Errorf("D4 = %+v", d4)
	}
}

// TestMariageAvantNaissance et compagnie reproduisent, en miniature, les cas
// documentés dans controle.py (Etienne Castel [I0250] pour R1, Paul/Bernard Denat
// pour R4/R5).
func TestMariageRecopieDesParents(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Père /Castel/\n1 FAMS @F0000@\n"+
		"0 @I0002@ INDI\n1 NAME Mère /Untel/\n1 FAMS @F0000@\n"+
		"0 @I0003@ INDI\n1 NAME Etienne /Castel/\n1 FAMC @F0000@\n1 FAMS @F0001@\n"+
		"0 @F0000@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 CHIL @I0003@\n1 MARR\n2 DATE 1 JAN 1700\n"+
		"0 @F0001@ FAM\n1 HUSB @I0003@\n1 MARR\n2 DATE 1 JAN 1700\n"+
		piedMinimal)
	r1 := R1(g, Seuils{})
	if len(r1) != 1 {
		t.Errorf("R1 = %+v", r1)
	}
}

func TestMariageAvantNaissance(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 BIRT\n2 DATE 1700\n1 FAMS @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 MARR\n2 DATE 1695\n"+
		piedMinimal)
	r2 := R2(g, config.Defauts())
	if len(r2) != 1 {
		t.Errorf("R2 = %+v", r2)
	}
}

func TestLongeviteInvraisemblable(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 BIRT\n2 DATE 1700\n1 DEAT\n2 DATE 1820\n"+
		piedMinimal)
	r3 := R3(g, config.Defauts())
	if len(r3) != 1 {
		t.Errorf("R3 = %+v", r3)
	}
}

func TestEcartEpoux(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 BIRT\n2 DATE 1700\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Marie /Dupont/\n1 BIRT\n2 DATE 1750\n1 FAMS @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n"+
		piedMinimal)
	r6 := R6(g, config.Defauts())
	if len(r6) != 1 {
		t.Errorf("R6 = %+v", r6)
	}
}

func TestR7MereTropAgee(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Mère /Untel/\n1 BIRT\n2 DATE 1900\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Enfant /Untel/\n1 BIRT\n2 DATE 1960\n1 FAMC @F0001@\n"+ // mère à 60 ans
		"0 @F0001@ FAM\n1 WIFE @I0001@\n1 CHIL @I0002@\n"+
		piedMinimal)
	if r7 := R7(g, config.Defauts()); len(r7) != 1 {
		t.Errorf("R7 = %+v", r7)
	}
}

func TestR8PereTropAge(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Père /Untel/\n1 BIRT\n2 DATE 1900\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Enfant /Untel/\n1 BIRT\n2 DATE 1985\n1 FAMC @F0001@\n"+ // père à 85 ans
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 CHIL @I0002@\n"+
		piedMinimal)
	if r8 := R8(g, config.Defauts()); len(r8) != 1 {
		t.Errorf("R8 = %+v", r8)
	}
}

func TestR9GermainsTropRapproches(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Aîné /Untel/\n1 BIRT\n2 DATE 1 JAN 2000\n1 FAMC @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Cadet /Untel/\n1 BIRT\n2 DATE 1 MAY 2000\n1 FAMC @F0001@\n"+ // 4 mois plus tard
		"0 @F0001@ FAM\n1 CHIL @I0001@\n1 CHIL @I0002@\n"+
		piedMinimal)
	if r9 := R9(g, config.Defauts()); len(r9) != 1 {
		t.Errorf("R9 = %+v", r9)
	}
}

func TestR9JumeauxToleres(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jumeau1 /Untel/\n1 BIRT\n2 DATE 1 JAN 2000\n1 FAMC @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Jumeau2 /Untel/\n1 BIRT\n2 DATE 1 JAN 2000\n1 FAMC @F0001@\n"+ // même jour
		"0 @F0001@ FAM\n1 CHIL @I0001@\n1 CHIL @I0002@\n"+
		piedMinimal)
	if r9 := R9(g, config.Defauts()); len(r9) != 0 {
		t.Errorf("R9 : jumeaux auraient dû être tolérés, obtenu %+v", r9)
	}
}

func TestR10MariageApresDeces(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 DEAT\n2 DATE 1700\n1 FAMS @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 MARR\n2 DATE 1710\n"+
		piedMinimal)
	if r10 := R10(g, config.Defauts()); len(r10) != 1 {
		t.Errorf("R10 = %+v", r10)
	}
}

func TestR11DateDansLeFutur(t *testing.T) {
	// gedcom.Annee ne reconnaît que 1000-2099 (voir date.go) : une année au-delà de
	// 2026 mais dans cette plage suffit à tester "dans le futur" sans dépendre de la
	// date du jour au-delà de la décennie courante.
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 BIRT\n2 DATE 1 JAN 2090\n"+
		piedMinimal)
	if r11 := R11(g, config.Defauts()); len(r11) != 1 {
		t.Errorf("R11 = %+v", r11)
	}
}

func TestR12OrdreIncoherent(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 BIRT\n2 DATE 1800\n1 BAPM\n2 DATE 1799\n"+
		piedMinimal)
	if r12 := R12(g, config.Defauts()); len(r12) != 1 {
		t.Errorf("R12 = %+v", r12)
	}
}

func TestR13AucunDeces(t *testing.T) {
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 BIRT\n2 DATE 1800\n"+
		piedMinimal)
	if r13 := R13(g, config.Defauts()); len(r13) != 1 {
		t.Errorf("R13 = %+v", r13)
	}

	// "DEAT Y" (décès présumé, sans date) doit être traité comme "décès connu".
	g2 := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 BIRT\n2 DATE 1800\n1 DEAT Y\n"+
		piedMinimal)
	if r13 := R13(g2, config.Defauts()); len(r13) != 0 {
		t.Errorf("R13 : DEAT Y aurait dû être traité comme décès connu, obtenu %+v", r13)
	}
}

func TestRegistreCompletSansFauxPositifSurUnArbrePropre(t *testing.T) {
	// Dates volontairement récentes (et non celles, XIXe siècle, des autres tests) :
	// un individu sans DEAT né il y a plus de LongeviteMax ans déclenche R13 à bon
	// droit (voir TestR13AucunDeces plus bas) — ce n'est pas un cas pour "arbre propre".
	g := charger(t, enteteMinimale+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 SEX M\n1 BIRT\n2 DATE 1 JAN 1970\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Marie /Dupont/\n1 SEX F\n1 BIRT\n2 DATE 2 FEB 1972\n1 FAMS @F0001@\n"+
		"0 @I0003@ INDI\n1 NAME Paul /Untel/\n1 SEX M\n1 BIRT\n2 DATE 3 MAR 1995\n1 FAMC @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 CHIL @I0003@\n1 MARR\n2 DATE 1 JUN 1990\n"+
		piedMinimal)
	for _, r := range Registre {
		if got := r.Verifie(g, config.Defauts()); len(got) != 0 {
			t.Errorf("%s : faux positif sur un arbre propre : %+v", r.ID, got)
		}
	}
}

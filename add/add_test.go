package add

import (
	"os"
	"path/filepath"
	"testing"

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

func TestAjoutSimple(t *testing.T) {
	g := charger(t, entete+pied)
	res, err := Ajouter(g, Requete{Nom: "Jean /Untel/", Sexe: "M", Naissance: "12 MAR 1805"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Xref != "I0000" {
		t.Errorf("xref = %q, voulu I0000", res.Xref)
	}
	indi, ok := g.Get(res.Xref)
	if !ok {
		t.Fatal("individu non créé")
	}
	if indi.Nom() != "Jean Untel" || indi.Date("BIRT") != "12 MAR 1805" {
		t.Errorf("individu créé incorrect : nom=%q naiss=%q", indi.Nom(), indi.Date("BIRT"))
	}
}

// TestLiensReciproques est le test central : un ajout avec père, mère et conjoint
// doit câbler les DEUX sens de chaque lien, pas seulement un.
func TestLiensReciproques(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Père /Untel/\n1 SEX M\n"+
		"0 @I0002@ INDI\n1 NAME Mère /Dupont/\n1 SEX F\n"+
		"0 @I0003@ INDI\n1 NAME Conjoint /Autre/\n1 SEX F\n"+
		pied)

	res, err := Ajouter(g, Requete{
		Nom: "Paul /Untel/", Sexe: "M",
		Pere: "I0001", Mere: "I0002", Conjoint: "I0003",
	})
	if err != nil {
		t.Fatal(err)
	}
	nouveau, _ := g.Get(res.Xref)

	// Sens INDI -> FAM
	famcs := nouveau.FamcPedi()
	if len(famcs) != 1 {
		t.Fatalf("FAMC = %+v", famcs)
	}
	famParent := famcs[0].Fam
	famsConjoint := nouveau.Valeurs("FAMS")
	if len(famsConjoint) != 1 {
		t.Fatalf("FAMS = %v", famsConjoint)
	}

	// Sens FAM -> INDI (parents)
	fp, _ := g.Get(famParent)
	if !contient(fp.Valeurs("CHIL"), res.Xref) {
		t.Errorf("FAM parent %s ne porte pas CHIL @%s@", famParent, res.Xref)
	}
	if fp.Valeur("HUSB") != "I0001" || fp.Valeur("WIFE") != "I0002" {
		t.Errorf("FAM parent : HUSB=%q WIFE=%q", fp.Valeur("HUSB"), fp.Valeur("WIFE"))
	}

	// Sens FAM -> INDI (conjoint) + réciprocité côté conjoint
	fc, _ := g.Get(famsConjoint[0])
	if fc.Valeur("HUSB") != res.Xref || fc.Valeur("WIFE") != "I0003" {
		t.Errorf("FAM conjoint : HUSB=%q WIFE=%q", fc.Valeur("HUSB"), fc.Valeur("WIFE"))
	}
	conjoint, _ := g.Get("I0003")
	if !contient(conjoint.Valeurs("FAMS"), famsConjoint[0]) {
		t.Errorf("I0003 ne porte pas FAMS réciproque vers %s", famsConjoint[0])
	}
}

func contient(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// TestReciprociteParentsNouvelleFamille reproduit le bug I0614/F0259 documenté dans
// controle_liens.py : quand une NOUVELLE famille est créée pour père+mère, ceux-ci
// doivent recevoir "1 FAMS" en retour, pas seulement porter HUSB/WIFE côté FAM.
func TestReciprociteParentsNouvelleFamille(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Père /Untel/\n1 SEX M\n"+
		"0 @I0002@ INDI\n1 NAME Mère /Dupont/\n1 SEX F\n"+
		pied)
	res, err := Ajouter(g, Requete{Nom: "Enfant /Untel/", Pere: "I0001", Mere: "I0002"})
	if err != nil {
		t.Fatal(err)
	}
	nouveau, _ := g.Get(res.Xref)
	famcs := nouveau.FamcPedi()
	if len(famcs) != 1 {
		t.Fatalf("FAMC = %+v", famcs)
	}
	fam := famcs[0].Fam

	pere, _ := g.Get("I0001")
	if !contient(pere.Valeurs("FAMS"), fam) {
		t.Errorf("le père ne porte pas `1 FAMS @%s@` en retour — bug I0614/F0259 reproduit", fam)
	}
	mere, _ := g.Get("I0002")
	if !contient(mere.Valeurs("FAMS"), fam) {
		t.Errorf("la mère ne porte pas `1 FAMS @%s@` en retour — bug I0614/F0259 reproduit", fam)
	}
}

func TestReutiliseFamilleExistante(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Père /Untel/\n1 SEX M\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Mère /Dupont/\n1 SEX F\n1 FAMS @F0001@\n"+
		"0 @I0003@ INDI\n1 NAME Aîné /Untel/\n1 FAMC @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 CHIL @I0003@\n"+
		pied)

	res, err := Ajouter(g, Requete{Nom: "Cadet /Untel/", Pere: "I0001", Mere: "I0002"})
	if err != nil {
		t.Fatal(err)
	}
	fam, _ := g.Get("F0001")
	if !contient(fam.Valeurs("CHIL"), res.Xref) || !contient(fam.Valeurs("CHIL"), "I0003") {
		t.Errorf("F0001 devrait porter les deux enfants : %v", fam.Valeurs("CHIL"))
	}
	if len(g.Familles()) != 1 {
		t.Errorf("une nouvelle famille a été créée à tort : %d familles", len(g.Familles()))
	}
}

func TestHomonymeBloqueSaufForce(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 BIRT\n2 DATE 1805\n"+pied)

	_, err := Ajouter(g, Requete{Nom: "Jean /Untel/", Naissance: "1806"})
	if err == nil {
		t.Fatal("attendu ErrHomonymes")
	}
	if _, ok := err.(*ErrHomonymes); !ok {
		t.Errorf("erreur inattendue : %T %v", err, err)
	}

	res, err := Ajouter(g, Requete{Nom: "Jean /Untel/", Naissance: "1806", IgnorerHomonymes: true})
	if err != nil {
		t.Fatalf("--force aurait dû passer outre : %v", err)
	}
	if len(res.Homonymes) != 1 {
		t.Errorf("Homonymes = %+v", res.Homonymes)
	}
}

func TestHomonymeInsensibleAuxAccents(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Gélis /Azéma/\n"+pied)
	homonymes := ChercherHomonymes(g, Requete{Nom: "GELIS /AZEMA/"})
	if len(homonymes) != 1 {
		t.Errorf("Homonymes = %+v", homonymes)
	}
}

func TestPereInconnuRefuse(t *testing.T) {
	g := charger(t, entete+pied)
	if _, err := Ajouter(g, Requete{Nom: "Jean /Untel/", Pere: "I9999"}); err == nil {
		t.Error("attendu une erreur : père inexistant")
	}
}

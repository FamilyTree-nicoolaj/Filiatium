package merge

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

func TestCollisionsDeXref(t *testing.T) {
	base := charger(t, entete+"0 @I0001@ INDI\n1 NAME Jean /Untel/\n"+pied)
	apport := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Un /Autre/\n"+ // collision
		"0 @I0002@ INDI\n1 NAME Sans /Collision/\n"+
		pied)
	c := detecterCollisions(base, apport)
	if c.Individus != 1 {
		t.Errorf("collisions individus = %d, voulu 1", c.Individus)
	}
}

func TestApparieHomonymeCertain(t *testing.T) {
	base := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 SEX M\n1 BIRT\n2 DATE 12 MAR 1805\n"+pied)
	apport := charger(t, entete+
		"0 @B0001@ INDI\n1 NAME Jean /Untel/\n1 SEX M\n1 BIRT\n2 DATE 1805\n"+pied)

	appariements := apparier(base, apport)
	if len(appariements) != 1 {
		t.Fatalf("appariements = %+v", appariements)
	}
	a := appariements[0]
	if a.Classe != Certaine {
		t.Errorf("classe = %s, score = %d, voulu certaine", a.Classe, a.Score)
	}
	if a.XrefBase != "I0001" || a.XrefApport != "B0001" {
		t.Errorf("appariement = %+v", a)
	}
}

func TestAucunAppariementPatronymeDifferent(t *testing.T) {
	base := charger(t, entete+"0 @I0001@ INDI\n1 NAME Jean /Untel/\n"+pied)
	apport := charger(t, entete+"0 @B0001@ INDI\n1 NAME Jean /Autrechose/\n"+pied)
	if a := apparier(base, apport); len(a) != 0 {
		t.Errorf("appariement inattendu : %+v", a)
	}
}

func TestConflitSexeDifferent(t *testing.T) {
	base := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 SEX M\n1 BIRT\n2 DATE 1805\n"+pied)
	apport := charger(t, entete+
		"0 @B0001@ INDI\n1 NAME Jean /Untel/\n1 SEX F\n1 BIRT\n2 DATE 1805\n"+pied)
	appariements := apparier(base, apport)
	if len(appariements) != 1 {
		t.Fatalf("appariements = %+v", appariements)
	}
	if len(appariements[0].Conflits) == 0 {
		t.Error("conflit de sexe non détecté")
	}
	if appariements[0].Classe != AExaminer {
		t.Errorf("classe = %s, voulu à examiner (conflit présent)", appariements[0].Classe)
	}
}

func TestBonusParente(t *testing.T) {
	// Deux arbres où le patronyme+prénom seuls ne suffiraient qu'à "probable" ;
	// la concordance des parents doit faire franchir le seuil "certaine".
	base := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Pierre /Untel/\n1 BIRT\n2 DATE 1770\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Paul /Untel/\n1 FAMC @F0001@\n1 BIRT\n2 DATE 1800\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 CHIL @I0002@\n"+
		pied)
	apport := charger(t, entete+
		"0 @B0001@ INDI\n1 NAME Pierre /Untel/\n1 BIRT\n2 DATE 1770\n1 FAMS @BF01@\n"+
		"0 @B0002@ INDI\n1 NAME Paul /Untel/\n1 FAMC @BF01@\n1 BIRT\n2 DATE\n"+ // date de naissance inconnue côté apport
		"0 @BF01@ FAM\n1 HUSB @B0001@\n1 CHIL @B0002@\n"+
		pied)

	// Pierre(base) est aussi candidat face à Paul(apport) — même patronyme, prénom
	// différent — d'où la recherche par (XrefBase, XrefApport) précis plutôt que par
	// XrefApport seul.
	appariements := apparier(base, apport)
	var paul *Appariement
	for i := range appariements {
		if appariements[i].XrefBase == "I0002" && appariements[i].XrefApport == "B0002" {
			paul = &appariements[i]
		}
	}
	if paul == nil {
		t.Fatalf("paire I0002<->B0002 absente des appariements : %+v", appariements)
	}
	trouve := false
	for _, c := range paul.Criteres {
		if c == "père déjà apparié" {
			trouve = true
		}
	}
	if !trouve {
		t.Errorf("bonus de parenté non appliqué : %+v", paul)
	}
}

func TestRenumeroterReecritLesPointeurs(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 FAMS @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n"+
		pied)
	r := renumeroter(g, "B")
	i1, ok := r.Get("BI0001")
	if !ok {
		t.Fatal("I0001 non renuméroté en BI0001")
	}
	if got := i1.Valeur("FAMS"); got != "BF0001" {
		t.Errorf("pointeur FAMS non réécrit : %q", got)
	}
	f1, ok := r.Get("BF0001")
	if !ok {
		t.Fatal("F0001 non renuméroté")
	}
	if got := f1.Valeur("HUSB"); got != "BI0001" {
		t.Errorf("pointeur HUSB non réécrit : %q", got)
	}
}

func TestPlanEstApplicable(t *testing.T) {
	base := charger(t, entete+"0 @I0001@ INDI\n1 NAME Jean /Untel/\n"+pied)
	apport := charger(t, entete+"0 @I0001@ INDI\n1 NAME Autre /Personne/\n"+pied) // collision volontaire

	plan := Plan(base, apport, "base.ged", "B")
	if len(plan.Operations) != 1 {
		t.Fatalf("plan.Operations = %+v", plan.Operations)
	}
	if err := plan.Appliquer(base); err != nil {
		t.Fatal(err)
	}
	if !base.Contains("BI0001") {
		t.Error("le plan appliqué n'a pas inséré BI0001")
	}
	if base.Contains("I0001") == false {
		t.Error("I0001 original toujours présent (attendu)")
	}
}

func TestVerdictFusionnableTelQuel(t *testing.T) {
	base := charger(t, entete+"0 @I0001@ INDI\n1 NAME Jean /Untel/\n"+pied)
	apport := charger(t, entete+"0 @A0001@ INDI\n1 NAME Sans /Rapport/\n"+pied)
	a := Analyser(base, apport)
	if a.Collisions.Total() != 0 {
		t.Errorf("collisions inattendues : %+v", a.Collisions)
	}
	if len(a.NouveauxApresMerge) != 0 {
		t.Errorf("contradictions inattendues : %v", a.NouveauxApresMerge)
	}
}

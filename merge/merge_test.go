package merge

import (
	"os"
	"path/filepath"
	"strings"
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

func TestScoreConflitsElimineLeRapprochementAbsurde(t *testing.T) {
	// Même patronyme et rien d'autre en commun : prénom, naissance et sexe divergent
	// tous les trois. Les conflits doivent retrancher assez de points pour que la
	// paire n'atteigne même plus le seuil d'affichage.
	base := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Marguerite /Le Gougne/\n1 SEX F\n1 BIRT\n2 DATE 1772\n"+pied)
	apport := charger(t, entete+
		"0 @B0001@ INDI\n1 NAME Tim /Le Gougne/\n1 SEX M\n1 BIRT\n2 DATE 2018\n"+pied)

	if a := apparier(base, apport); len(a) != 0 {
		t.Errorf("rapprochement absurde non éliminé : %+v", a)
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

func TestFusionReutiliseLesEnregistrementsIdentiques(t *testing.T) {
	base := charger(t, entete+
		"0 @S0001@ SOUR\n1 TITL Recensement 1911\n"+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n"+pied)
	apport := charger(t, entete+
		"0 @S0099@ SOUR\n1 TITL Recensement 1911\n"+ // même contenu, xref différent
		"0 @B0001@ INDI\n1 NAME Sans /Rapport/\n"+pied)

	f := preparer(base, apport, NiveauCertaines)
	if xb, ok := f.apparies["S0099"]; !ok || xb != "S0001" {
		t.Errorf("S0099 non identifié à S0001 par contenu : %+v", f.apparies)
	}
	for _, c := range f.copies {
		if c.Tag == "SOUR" {
			t.Errorf("SOUR dupliquée dans les copies : %+v", c)
		}
	}
	if len(f.copies) != 1 { // seul l'INDI "Sans /Rapport/" doit être copié
		t.Errorf("copies = %d, voulu 1 : %+v", len(f.copies), f.copies)
	}
}

func TestFusionCompleteUneFamille(t *testing.T) {
	// Cas réel (F0049) : les deux exports partagent HUSB/WIFE/un enfant à l'identique
	// (mêmes xref, même contenu — identifiés par contenu, pas par accord de xref),
	// mais l'apport porte un enfant supplémentaire, absent de la base.
	base := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 SEX M\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Marie /Autre/\n1 SEX F\n1 FAMS @F0001@\n"+
		"0 @I0003@ INDI\n1 NAME Paul /Untel/\n1 FAMC @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 CHIL @I0003@\n"+
		pied)
	apport := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 SEX M\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Marie /Autre/\n1 SEX F\n1 FAMS @F0001@\n"+
		"0 @I0003@ INDI\n1 NAME Paul /Untel/\n1 FAMC @F0001@\n"+
		"0 @A4@ INDI\n1 NAME Sophie /Untel/\n1 FAMC @F0001@\n"+ // enfant supplémentaire
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 CHIL @I0003@\n1 CHIL @A4@\n"+
		pied)

	f := preparer(base, apport, NiveauCertaines)
	if xb, ok := f.apparies["F0001"]; !ok || xb != "F0001" {
		t.Fatalf("famille d'apport non appariée à celle de la base : %+v", f.apparies)
	}
	ajouts := f.completes["F0001"]
	voulu := "1 CHIL @" + f.table["A4"] + "@"
	if len(ajouts) != 1 || ajouts[0] != voulu {
		t.Errorf("lignes ajoutées = %v, voulu [%q]", ajouts, voulu)
	}
	for _, c := range f.copies {
		if c.Tag == "FAM" {
			t.Errorf("famille dupliquée alors qu'elle aurait dû être complétée : %+v", c)
		}
	}
}

func TestFusionFamilleSansConjointCoteApport(t *testing.T) {
	// Cas réel (F0127) : l'apport a perdu HUSB/WIFE, ne conserve que le lien vers
	// l'enfant — la famille doit malgré tout être reconnue comme la même, et la base
	// (déjà plus riche) ne doit rien recevoir.
	base := charger(t, entete+
		"0 @I0010@ INDI\n1 NAME Henri /Dupont/\n1 SEX M\n1 FAMS @F0002@\n"+
		"0 @I0011@ INDI\n1 NAME Alice /Voisin/\n1 SEX F\n1 FAMS @F0002@\n"+
		"0 @I0012@ INDI\n1 NAME Robert /Dupont/\n1 FAMC @F0002@\n"+
		"0 @F0002@ FAM\n1 HUSB @I0010@\n1 WIFE @I0011@\n1 CHIL @I0012@\n"+
		pied)
	apport := charger(t, entete+
		"0 @I0012@ INDI\n1 NAME Robert /Dupont/\n1 FAMC @F0002@\n"+
		"0 @F0002@ FAM\n1 CHIL @I0012@\n"+
		pied)

	f := preparer(base, apport, NiveauCertaines)
	if xb, ok := f.apparies["F0002"]; !ok || xb != "F0002" {
		t.Fatalf("famille (sans conjoint côté apport) non appariée : %+v", f.apparies)
	}
	if len(f.copies) != 0 {
		t.Errorf("copies inattendues : %+v", f.copies)
	}
	if len(f.completes) != 0 {
		t.Errorf("complétions inattendues (la base est déjà plus riche) : %+v", f.completes)
	}
}

func TestFusionRenumeroteSeulementEnCasDeCollision(t *testing.T) {
	base := charger(t, entete+"0 @I0001@ INDI\n1 NAME Jean /Untel/\n"+pied)
	apport := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Autre /Personne/\n"+ // collision de xref, aucun rapport de contenu
		"0 @I0099@ INDI\n1 NAME Libre /Xref/\n"+ // xref libre côté base
		pied)

	f := preparer(base, apport, NiveauCertaines)
	if len(f.copies) != 2 {
		t.Fatalf("copies = %d, voulu 2 : %+v", len(f.copies), f.copies)
	}
	nouveau, renumerote := f.table["I0001"], f.renumerotes["I0001"]
	if renumerote == "" || nouveau != renumerote || nouveau == "I0001" {
		t.Errorf("I0001 (apport, en collision) non renuméroté : table=%q renumerotes=%q", nouveau, renumerote)
	}
	if got := f.table["I0099"]; got != "I0099" {
		t.Errorf("I0099 (xref libre) aurait dû être conservé, obtenu %q", got)
	}
}

func TestSignaturesDupliqueesInterneNeCassentRien(t *testing.T) {
	base := charger(t, entete+
		"0 @N1@ NOTE Texte commun\n"+
		"0 @N2@ NOTE Texte commun\n"+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 NOTE @N1@\n"+
		"0 @I0002@ INDI\n1 NAME Marc /Untel/\n1 NOTE @N2@\n"+
		pied)
	apport := charger(t, entete+
		"0 @M1@ NOTE Texte commun\n"+
		"0 @M2@ NOTE Texte commun\n"+
		"0 @A1@ INDI\n1 NAME Alice /Voisin/\n1 NOTE @M1@\n"+
		"0 @A2@ INDI\n1 NAME Bob /Voisin/\n1 NOTE @M2@\n"+
		pied)

	f := preparer(base, apport, NiveauCertaines)
	notesAppariees := 0
	for x, xb := range f.apparies {
		if x == "M1" || x == "M2" {
			notesAppariees++
			if xb != "N1" && xb != "N2" {
				t.Errorf("NOTE %s appariée à %s, voulu N1 ou N2", x, xb)
			}
		}
	}
	if notesAppariees != 2 {
		t.Fatalf("NOTE appariées = %d, voulu 2 : %+v", notesAppariees, f.apparies)
	}
	for _, c := range f.copies {
		if c.Tag == "NOTE" {
			t.Errorf("NOTE dupliquée en copie : %+v", c)
		}
		for _, l := range c.Lignes {
			if strings.Contains(l, "@M1@") || strings.Contains(l, "@M2@") {
				t.Errorf("pointeur non résolu vers une note d'apport : %s", l)
			}
		}
	}
}

func TestBlocEnConflitNonApplique(t *testing.T) {
	base := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Marie /Autre/\n1 FAMS @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 MARR\n2 DATE 1 JAN 1920\n"+
		pied)
	apport := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Marie /Autre/\n1 FAMS @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 MARR\n2 DATE 5 MAY 1921\n"+ // date de mariage différente
		pied)

	f := preparer(base, apport, NiveauCertaines)
	if len(f.completes) != 0 {
		t.Errorf("aucune complétion attendue (bloc en conflit, pas d'ajout automatique) : %+v", f.completes)
	}
	if len(f.conflits) != 1 {
		t.Fatalf("conflits = %d, voulu 1 : %v", len(f.conflits), f.conflits)
	}
}

func TestNiveauIdentiquesNeFusionnePasLesFamilles(t *testing.T) {
	base := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 SEX M\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Marie /Autre/\n1 SEX F\n1 FAMS @F0001@\n"+
		"0 @I0003@ INDI\n1 NAME Paul /Untel/\n1 FAMC @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 CHIL @I0003@\n"+
		pied)
	apport := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 SEX M\n1 FAMS @F0001@\n"+
		"0 @I0002@ INDI\n1 NAME Marie /Autre/\n1 SEX F\n1 FAMS @F0001@\n"+
		"0 @I0003@ INDI\n1 NAME Paul /Untel/\n1 FAMC @F0001@\n"+
		"0 @A4@ INDI\n1 NAME Sophie /Untel/\n1 FAMC @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n1 WIFE @I0002@\n1 CHIL @I0003@\n1 CHIL @A4@\n"+
		pied)

	f := preparer(base, apport, NiveauIdentiques)
	if _, ok := f.apparies["F0001"]; ok {
		t.Errorf("famille appariée alors que le niveau est \"identiques\" : %+v", f.apparies)
	}
	trouveFAM := false
	for _, c := range f.copies {
		if c.Tag != "FAM" {
			continue
		}
		trouveFAM = true
		if c.Xref == "F0001" {
			t.Errorf("famille copiée sans renumérotation malgré la collision de xref : %+v", c)
		}
	}
	if !trouveFAM {
		t.Errorf("famille non copiée : %+v", f.copies)
	}
}

func TestReecrireReecritLesPointeurs(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 FAMS @F0001@\n"+
		"0 @F0001@ FAM\n1 HUSB @I0001@\n"+
		pied)
	table := map[string]string{"I0001": "BI0001", "F0001": "BF0001"}
	r := reecrire(g, table)
	i1, ok := r.Get("BI0001")
	if !ok {
		t.Fatal("I0001 non réécrit en BI0001")
	}
	if got := i1.Valeur("FAMS"); got != "BF0001" {
		t.Errorf("pointeur FAMS non réécrit : %q", got)
	}
	f1, ok := r.Get("BF0001")
	if !ok {
		t.Fatal("F0001 non réécrit")
	}
	if got := f1.Valeur("HUSB"); got != "BI0001" {
		t.Errorf("pointeur HUSB non réécrit : %q", got)
	}
}

func TestPlanEstApplicable(t *testing.T) {
	base := charger(t, entete+"0 @I0001@ INDI\n1 NAME Jean /Untel/\n"+pied)
	apport := charger(t, entete+"0 @I0001@ INDI\n1 NAME Autre /Personne/\n"+pied) // collision volontaire, aucun rapport de contenu

	plan := Plan(base, apport, "base.ged", NiveauCertaines)
	if len(plan.Operations) != 1 {
		t.Fatalf("plan.Operations = %+v", plan.Operations)
	}
	if err := plan.Appliquer(base); err != nil {
		t.Fatal(err)
	}
	if !base.Contains("I0002") {
		t.Error("le plan appliqué n'a pas inséré le xref renuméroté I0002")
	}
	if !base.Contains("I0001") {
		t.Error("I0001 original toujours présent (attendu)")
	}
}

func TestVerdictFusionnableTelQuel(t *testing.T) {
	base := charger(t, entete+"0 @I0001@ INDI\n1 NAME Jean /Untel/\n"+pied)
	apport := charger(t, entete+"0 @A0001@ INDI\n1 NAME Sans /Rapport/\n"+pied)
	a := Analyser(base, apport, NiveauCertaines)
	if a.Renumerotes != 0 {
		t.Errorf("renumérotations inattendues : %d", a.Renumerotes)
	}
	if len(a.NouveauxApresMerge) != 0 {
		t.Errorf("contradictions inattendues : %v", a.NouveauxApresMerge)
	}
}

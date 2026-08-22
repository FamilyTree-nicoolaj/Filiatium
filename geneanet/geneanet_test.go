package geneanet

import (
	"os"
	"testing"

	"github.com/FamilyTree-nicoolaj/filiatium/add"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

func lireFixture(t *testing.T, nom string) []byte {
	t.Helper()
	contenu, err := os.ReadFile("testdata/" + nom)
	if err != nil {
		t.Fatal(err)
	}
	return contenu
}

func TestEstFicheGeneanet(t *testing.T) {
	if !EstFicheGeneanet(lireFixture(t, "fiche_sujet.html")) {
		t.Error("fiche_sujet.html devrait être reconnue")
	}
	if EstFicheGeneanet([]byte("<html><body>rien à voir</body></html>")) {
		t.Error("une page sans gw.geneanet.org ne devrait pas être reconnue")
	}
}

// TestParserHTMLFicheSujet couvre la forme générale d'une fiche Geneanet réelle
// (données inventées, dépôt public — voir testdata/fiche_sujet.html) : sujet,
// naissance/décès/profession, Parents, Frères et sœurs (dont l'auto-référence en <b>,
// sans lien vers soi-même), Notes, Sources, et l'arbre des 4 grands-parents directs
// (sans oncles/tantes — cette vue simple ne les montre pas, jamais deviné).
func TestParserHTMLFicheSujet(t *testing.T) {
	f, err := ParserHTML(lireFixture(t, "fiche_sujet.html"))
	if err != nil {
		t.Fatal(err)
	}

	if f.Sujet.Nom != "Julien Fabre" || f.Sujet.Sexe != "M" {
		t.Fatalf("sujet = %+v", f.Sujet)
	}
	if f.Naissance.Date != "12 MAR 1810" || f.Naissance.Lieu != "Sarlat, 24001, Dordogne, Aquitaine, France" {
		t.Fatalf("naissance = %+v", f.Naissance)
	}
	if f.Deces.Date != "4 JAN 1880" || f.Deces.Lieu != "Sarlat, 24001, Dordogne, Aquitaine, France" {
		t.Fatalf("deces = %+v", f.Deces)
	}
	if f.Profession == nil || f.Profession.Intitule != "Meunier" || f.Profession.Lieu != "Sarlat" {
		t.Fatalf("profession = %+v", f.Profession)
	}

	if f.Parents[0].Nom != "Antoine Fabre" || f.Parents[0].Naissance != 1780 || f.Parents[0].Deces != 1850 {
		t.Fatalf("père = %+v", f.Parents[0])
	}
	if f.Parents[1].Nom != "Jeanne Vidal" || f.Parents[1].Naissance != 1782 || f.Parents[1].Deces != 1845 {
		t.Fatalf("mère = %+v", f.Parents[1])
	}

	if len(f.Fratrie) != 3 {
		t.Fatalf("fratrie = %+v", f.Fratrie)
	}
	soiMeme := f.Fratrie[1]
	if soiMeme.Nom != "Julien Fabre" || soiMeme.Sexe != "M" || soiMeme.Naissance != 1810 || soiMeme.Deces != 1880 {
		t.Fatalf("auto-référence (<b>, sans lien) = %+v", soiMeme)
	}
	louis := f.Fratrie[2]
	if louis.Nom != "Louis Fabre" || !louis.OkNaissance || louis.Naissance != 1813 || louis.OkDeces {
		t.Fatalf("Louis Fabre (1813-, décès inconnu) = %+v", louis)
	}

	if len(f.Notes) != 2 {
		t.Fatalf("notes = %+v", f.Notes)
	}
	if f.Notes[0] != "Naissance : déclaration le 13 mars, présents Jean Delmas cultivateur et Pierre Auriac meunier" {
		t.Errorf("note naissance = %q", f.Notes[0])
	}
	if f.Notes[1] != "Décès : orthographié aussi Fabres" {
		t.Errorf("note décès = %q", f.Notes[1])
	}

	if len(f.Sources) != 2 || f.Sources[0].Label != "Naissance" || f.Sources[0].Texte != "AD 24 Sarlat" {
		t.Fatalf("sources = %+v", f.Sources)
	}

	gp := f.GrandsParentsPaternels
	if gp == nil {
		t.Fatal("GrandsParentsPaternels manquant")
	}
	// Mathieu Fabre a une vignette photo dans la fixture (voir testdata) : sa cellule
	// porte donc DEUX <a> (le premier n'entourant qu'une <img>, texte vide) — vérifie
	// que nomNoeud saute bien le premier au profit de celui qui porte le nom.
	if gp.GrandPere.Nom != "Mathieu Fabre" || gp.GrandPere.Naissance != 1750 || gp.GrandPere.Deces != 1820 {
		t.Fatalf("grand-père paternel = %+v", gp.GrandPere)
	}
	if gp.GrandMere.Nom != "Catherine Roux" || gp.GrandMere.Naissance != 1755 || gp.GrandMere.Deces != 1825 {
		t.Fatalf("grand-mère paternelle = %+v", gp.GrandMere)
	}
	if gp.Enfants != nil {
		t.Errorf("oncles/tantes paternels devraient être vides (vue simple) : %+v", gp.Enfants)
	}

	gm := f.GrandsParentsMaternels
	if gm == nil {
		t.Fatal("GrandsParentsMaternels manquant")
	}
	if gm.GrandPere.Nom != "Pierre Vidal" || !gm.GrandPere.OkNaissance || gm.GrandPere.Naissance != 1748 ||
		!gm.GrandPere.OkDeces || gm.GrandPere.Deces != 1811 {
		t.Fatalf("grand-père maternel (« ca 1748-1811 ») = %+v", gm.GrandPere)
	}
	if gm.GrandMere.Nom != "Anne Granier" || gm.GrandMere.OkNaissance || gm.GrandMere.OkDeces {
		t.Fatalf("grand-mère maternelle (décès connu, date inconnue « † ») = %+v", gm.GrandMere)
	}
}

// TestParserHTMLUnion couvre la section Union(s) et enfant(s) — le point qui a motivé
// le passage à l'import HTML (l'OCR fusionnait deux personnes distinctes issues d'une
// mise en page à deux colonnes ; ici, chaque conjoint/enfant est dans sa propre
// balise, aucune fusion possible) — et la note d'union associée.
func TestParserHTMLUnion(t *testing.T) {
	f, err := ParserHTML(lireFixture(t, "fiche_union.html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Unions) != 1 {
		t.Fatalf("unions = %+v", f.Unions)
	}
	u := f.Unions[0]
	if u.Mariage.Date != "9 JUN 1803" {
		t.Errorf("date de mariage = %q", u.Mariage.Date)
	}
	// Le HTML laisse une virgule de fin ("...France, avec") qu'un OCR n'aurait jamais
	// produite — regression sur le lieu si elle n'est pas retirée.
	if u.Mariage.Lieu != "Sarlat, 24001, Dordogne, Aquitaine, France" {
		t.Errorf("lieu de mariage = %q (virgule de fin non retirée ?)", u.Mariage.Lieu)
	}
	if u.Conjoint.Nom != "Jeanne Vidal" || u.Conjoint.Naissance != 1782 || u.Conjoint.Deces != 1845 {
		t.Fatalf("conjoint = %+v", u.Conjoint)
	}
	if u.Note != "acte retrouvé aux registres paroissiaux, page 42" {
		t.Errorf("note d'union = %q", u.Note)
	}

	if len(u.Enfants) != 5 {
		t.Fatalf("enfants = %+v", u.Enfants)
	}
	rose := u.Enfants[3]
	if rose.Nom != "Rose Fabre" || rose.OkNaissance || !rose.OkDeces || rose.Deces != 1816 {
		t.Fatalf("Rose Fabre (†1816, décès seul daté) = %+v", rose)
	}
	paul := u.Enfants[4]
	if paul.Nom != "Paul Fabre" || paul.OkNaissance || paul.OkDeces {
		t.Fatalf("Paul Fabre (†, décès connu sans date) = %+v", paul)
	}
}

// TestConstruireGrandParentInconnu couvre le cas (réel, cf. import "sarraute75" sur
// une fiche Pierre Marquet) où un seul des deux grands-parents d'un groupe est
// nommé — Geneanet l'affiche ainsi quand l'autre est inconnu. cablerGroupeGrandsParents
// résolvait autrefois GrandPere/GrandMere sans vérifier leur Nom, contrairement à
// Parents (cablerParentsEtFratrie) et parentDuSujet : "mention sans nom" faisait
// échouer tout l'import.
func TestConstruireGrandParentInconnu(t *testing.T) {
	f := &Fiche{
		Sujet: Personne{Nom: "Pierre Marquet", Sexe: "M"},
		GrandsParentsPaternels: &GrandParentGroupe{
			GrandPere: Personne{Nom: "Maurice Marquet"},
			// GrandMere inconnue (Nom == "") — ne doit pas faire échouer Construire.
		},
	}
	g := gedcom.Nouveau()
	if _, err := Construire(g, []*Fiche{f}, ""); err != nil {
		t.Fatal(err)
	}
	gp := add.ChercherHomonymes(g, add.Requete{Nom: "Maurice /Marquet/"})
	if len(gp) != 1 {
		t.Fatalf("grand-père paternel : %d trouvé(s), attendu 1", len(gp))
	}
}

// TestConstruireFicheSujet couvre Construire au niveau de fiche_sujet.html (pas
// seulement le parsing) : le sujet est câblé sur la FAM de ses grands-parents
// paternels bien qu'absent de leur liste d'enfants (elle est toujours vide, voir
// TestParserHTMLFicheSujet), FAMC/CHIL réciproques des deux côtés.
func TestConstruireFicheSujet(t *testing.T) {
	f, err := ParserHTML(lireFixture(t, "fiche_sujet.html"))
	if err != nil {
		t.Fatal(err)
	}
	g := gedcom.Nouveau()
	if _, err := Construire(g, []*Fiche{f}, ""); err != nil {
		t.Fatal(err)
	}

	pere := add.ChercherHomonymes(g, add.Requete{Nom: "Antoine /Fabre/", Naissance: "1780"})
	if len(pere) != 1 {
		t.Fatalf("père du sujet : %d trouvé(s), attendu 1", len(pere))
	}
	pereRec, _ := g.Get(pere[0].Xref)
	gpFam := pereRec.Valeur("FAMC")
	if gpFam == "" {
		t.Fatal("le père du sujet n'a pas de FAMC vers la famille des grands-parents")
	}
	fam, _ := g.Get(gpFam)
	trouve := false
	for _, c := range fam.Valeurs("CHIL") {
		if c == pere[0].Xref {
			trouve = true
		}
	}
	if !trouve {
		t.Fatal("le père du sujet n'est pas CHIL de la FAM des grands-parents paternels")
	}
	if fam.Valeur("HUSB") == "" || fam.Valeur("WIFE") == "" {
		t.Fatalf("FAM des grands-parents paternels incomplète : %+v", fam.Lignes)
	}
}

// TestConstruireFichesReelles couvre la déduplication de Construire (même patronyme
// et prénom, et — quand connue des deux côtés — même année de naissance) sur 3 fiches
// qui se recoupent délibérément : Victoire LOUIS, son mari François Joseph BOUCHART,
// et la mère de celui-ci Marie Françoise TILMONT. Construit directement en Fiche{}
// littéraux (même contenu que les 3 fiches réelles ayant motivé cette vérification) :
// Construire est indépendant du format d'entrée (HTML ou, comme avant cette session,
// OCR), pas la peine d'une fixture HTML pour ce test.
func TestConstruireFichesReelles(t *testing.T) {
	prudent := Personne{Nom: "Prudent François BOUCHART", Sexe: "M", Naissance: 1826, OkNaissance: true, Deces: 1878, OkDeces: true}
	rosalie := Personne{Nom: "Rosalie Anastasie BOUCHART", Sexe: "F", Naissance: 1828, OkNaissance: true, Deces: 1900, OkDeces: true}
	augustineFille := Personne{Nom: "Augustine BOUCHART", Sexe: "F", Naissance: 1831, OkNaissance: true, Deces: 1916, OkDeces: true}

	fA := &Fiche{
		Sujet:     Personne{Nom: "Victoire LOUIS", Sexe: "F"},
		Naissance: Evenement{Date: "3 SEP 1798", Lieu: "Hasnon (Nord)"},
		Deces:     Evenement{Date: "26 APR 1856", Lieu: "Hasnon (Nord)"},
		Unions: []Union{{
			Conjoint: Personne{Nom: "François Joseph BOUCHART", Naissance: 1787, OkNaissance: true, Deces: 1870, OkDeces: true},
			Mariage:  Evenement{Date: "25 JUN 1822", Lieu: "Hasnon (Nord)"},
			Enfants:  []Personne{prudent, rosalie, augustineFille},
		}},
		Sources: []Source{{Label: "Famille", Texte: "AD du Nord"}},
	}

	fB := &Fiche{
		Sujet:     Personne{Nom: "François Joseph BOUCHART", Sexe: "M"},
		Naissance: Evenement{Date: "20 SEP 1787", Lieu: "Hasnon (Nord)"},
		Deces:     Evenement{Date: "10 SEP 1870", Lieu: "Hasnon (Nord)"},
		Parents: [2]Personne{
			{Nom: "Charles François Joseph BOUCHART", Naissance: 1754, OkNaissance: true, Deces: 1798, OkDeces: true},
			{Nom: "Augustine Françoise TILMONT", Naissance: 1760, OkNaissance: true, Deces: 1789, OkDeces: true},
		},
		Unions: []Union{{
			Conjoint: Personne{Nom: "Victoire LOUIS", Naissance: 1798, OkNaissance: true, Deces: 1856, OkDeces: true},
			Mariage:  Evenement{Date: "25 JUN 1822", Lieu: "Hasnon (Nord)"},
			Enfants:  []Personne{prudent, rosalie, augustineFille},
		}},
		Fratrie: []Personne{
			{Nom: "Marie Joseph BOUCHART", Sexe: "F", Naissance: 1785, OkNaissance: true, Deces: 1856, OkDeces: true},
			{Nom: "François Joseph BOUCHART", Sexe: "F", Naissance: 1786, OkNaissance: true, Deces: 1787, OkDeces: true},
			{Nom: "François Joseph BOUCHART", Sexe: "M", Naissance: 1787, OkNaissance: true, Deces: 1870, OkDeces: true},
			{Nom: "Marie Augustine BOUCHART", Sexe: "F", Naissance: 1789, OkNaissance: true},
		},
		DemiFratrie: []DemiFratrieGroupe{{
			ParentCommun: Personne{Nom: "Charles François Joseph BOUCHART", Naissance: 1754, OkNaissance: true, Deces: 1798, OkDeces: true},
			Unions: []Union{{
				Conjoint: Personne{Nom: "Marie Françoise TILMONT", Naissance: 1755, OkNaissance: true},
				Enfants: []Personne{
					{Nom: "Louis Joseph BOUCHART", Sexe: "M", Naissance: 1792, OkNaissance: true},
					{Nom: "Charles BOUCHART", Sexe: "M", Naissance: 1793, OkNaissance: true},
				},
			}},
		}},
	}

	fC := &Fiche{
		Sujet:     Personne{Nom: "Marie Françoise TILMONT", Sexe: "F"},
		Naissance: Evenement{Date: "26 NOV 1755", Lieu: "Hasnon (Nord)"},
		Parents: [2]Personne{
			{Nom: "Basile TILMONT"},
			{Nom: "Marie Françoise BAUDRY"},
		},
		Unions: []Union{
			{
				Conjoint: Personne{Nom: "Charles François Joseph BOUCHART", Naissance: 1754, OkNaissance: true, Deces: 1798, OkDeces: true},
				Mariage:  Evenement{Date: "7 FEB 1792", Lieu: "Hasnon (Nord)"},
				Enfants: []Personne{
					{Nom: "Louis Joseph BOUCHART", Sexe: "M", Naissance: 1792, OkNaissance: true},
					{Nom: "Charles BOUCHART", Sexe: "M", Naissance: 1793, OkNaissance: true},
				},
			},
			{
				Conjoint: Personne{Nom: "Charles SOIL", Deces: 1805, OkDeces: true},
				Mariage:  Evenement{Date: "30 JAN 1798", Lieu: "Millonfosse (Nord)"},
			},
		},
		Fratrie: []Personne{
			{Nom: "Jacques Philippe TILMONT", Sexe: "M", Naissance: 1753, OkNaissance: true},
			{Nom: "Marie Françoise TILMONT", Sexe: "F", Naissance: 1755, OkNaissance: true},
			{Nom: "Augustine Françoise TILMONT", Sexe: "F", Naissance: 1760, OkNaissance: true, Deces: 1789, OkDeces: true},
		},
		Sources: []Source{
			{Label: "Naissance", Texte: "AD du Nord p 353"},
			{Label: "Union 1", Texte: "AD du Nord p 180"},
			{Label: "Union 2", Texte: "AD du Nord p 78"},
		},
	}

	g := gedcom.Nouveau()
	rapport, err := Construire(g, []*Fiche{fA, fB, fC}, "Sylvie DUJARDIN (sylvied58)")
	if err != nil {
		t.Fatal(err)
	}

	// Individus attendus (dédupliqués) : Victoire, François Joseph, Prudent, Rosalie,
	// Augustine(fille), Charles(père), Augustine Françoise TILMONT(mère), Marie Joseph,
	// François Joseph(1786), Marie Augustine, Marie Françoise TILMONT, Louis Joseph,
	// Charles(1793), Basile TILMONT, Marie Françoise BAUDRY, Charles SOIL, Jacques
	// Philippe TILMONT = 17.
	if rapport.Individus != 17 {
		t.Errorf("individus = %d, attendu 17", rapport.Individus)
	}
	// Familles attendues : Victoire+François, parents de François (Charles+Augustine
	// F.), Charles+Marie Françoise TILMONT (union 1, = demi-fratrie), Charles+SOIL
	// (union 2 de Marie), parents de Marie (Basile+Baudry) = 5.
	if rapport.Familles != 5 {
		t.Errorf("familles = %d, attendu 5", rapport.Familles)
	}

	casDeduplication := []struct {
		nom       string
		naissance string
	}{
		{"Augustine Françoise /TILMONT/", "1760"},
		{"Charles François Joseph /BOUCHART/", "1754"},
		{"Marie Françoise /TILMONT/", "1755"},
	}
	for _, c := range casDeduplication {
		h := add.ChercherHomonymes(g, add.Requete{Nom: c.nom, Naissance: c.naissance})
		if len(h) != 1 {
			t.Errorf("%s (%s) : %d individu(s) trouvé(s), attendu 1 (%+v)", c.nom, c.naissance, len(h), h)
		}
	}
}

package geneanet

import (
	"testing"

	"github.com/FamilyTree-nicoolaj/filiatium/add"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

// Les 3 fiches réelles ayant motivé cette commande : Victoire LOUIS, son mari
// François Joseph BOUCHART, et la mère de celui-ci Marie Françoise TILMONT. Elles se
// recoupent délibérément (voir les 3 cas de déduplication testés plus bas).
const ficheVictoire = `
♀ Victoire LOUIS
- Née le 3 septembre 1798 (lundi) - Hasnon (Nord)
- Décédée le 26 avril 1856 (samedi) - Hasnon (Nord), à l'âge de 57 ans

Union(s) et enfant(s)
- Mariée le 25 juin 1822 (mardi), Hasnon (Nord), avec François Joseph BOUCHART 1787-1870 dont
    ♂ Prudent François BOUCHART 1826-1878
    ♀ Rosalie Anastasie BOUCHART 1828-1900
    ♀ Augustine BOUCHART 1831-1916

Sources
- Famille: AD du Nord
`

const ficheFrancois = `
♂ François Joseph BOUCHART
- Né le 20 septembre 1787 (jeudi) - Hasnon (Nord)
- Décédé le 10 septembre 1870 (samedi) - Hasnon (Nord), à l'âge de 82 ans

Parents
- Charles François Joseph BOUCHART 1754-1798
- Augustine Françoise TILMONT 1760-1789

Union(s) et enfant(s)
- Marié le 25 juin 1822 (mardi), Hasnon (Nord), avec Victoire LOUIS 1798-1856 dont
    ♂ Prudent François BOUCHART 1826-1878
    ♀ Rosalie Anastasie BOUCHART 1828-1900
    ♀ Augustine BOUCHART 1831-1916

Frères et sœurs
- ♀ Marie Joseph BOUCHART 1785-1856
- ♀ François Joseph BOUCHART 1786-1787
- ♂ François Joseph BOUCHART 1787-1870
- ♀ Marie Augustine BOUCHART 1789

Demi-frères et demi-sœurs
Du côté de Charles François Joseph BOUCHART 1754-1798
- avec Marie Françoise TILMONT 1755
    ♂ Louis Joseph BOUCHART 1792
    ♂ Charles BOUCHART 1793
`

const ficheMarie = `
♀ Marie Françoise TILMONT
- Née le 26 novembre 1755 (mercredi) - Hasnon (Nord)

Parents
- Basile TILMONT
- Marie Françoise BAUDRY

Union(s) et enfant(s)
- Mariée le 7 février 1792 (mardi), Hasnon (Nord), avec Charles François Joseph BOUCHART 1754-1798 dont
    ♂ Louis Joseph BOUCHART 1792
    ♂ Charles BOUCHART 1793
- Mariée le 30 janvier 1798 (mardi), Millonfosse (Nord), avec Charles SOIL †1805

Frères et sœurs
- ♂ Jacques Philippe TILMONT 1753
- ♀ Marie Françoise TILMONT 1755
- ♀ Augustine Françoise TILMONT 1760-1789

Sources
- Naissance: AD du Nord p 353
- Union 1: AD du Nord p 180
- Union 2: AD du Nord p 78
`

func TestParseFicheVictoire(t *testing.T) {
	f, err := Parse(ficheVictoire)
	if err != nil {
		t.Fatal(err)
	}
	if f.Sujet.Nom != "Victoire LOUIS" || f.Sujet.Sexe != "F" {
		t.Fatalf("sujet = %+v", f.Sujet)
	}
	if f.Naissance.Date != "3 SEP 1798" || f.Naissance.Lieu != "Hasnon (Nord)" {
		t.Fatalf("naissance = %+v", f.Naissance)
	}
	if f.Deces.Date != "26 APR 1856" || f.Deces.Lieu != "Hasnon (Nord)" {
		t.Fatalf("deces = %+v", f.Deces)
	}
	if len(f.Unions) != 1 || f.Unions[0].Conjoint.Nom != "François Joseph BOUCHART" {
		t.Fatalf("unions = %+v", f.Unions)
	}
	if f.Unions[0].Mariage.Date != "25 JUN 1822" || f.Unions[0].Mariage.Lieu != "Hasnon (Nord)" {
		t.Fatalf("mariage = %+v", f.Unions[0].Mariage)
	}
	if len(f.Unions[0].Enfants) != 3 {
		t.Fatalf("enfants = %+v", f.Unions[0].Enfants)
	}
	if len(f.Sources) != 1 || f.Sources[0].Label != "Famille" || f.Sources[0].Texte != "AD du Nord" {
		t.Fatalf("sources = %+v", f.Sources)
	}
}

// Fiche réelle montrant le bloc "Grands-parents, oncles et tantes" (2 générations en
// arrière), absent des 3 fiches ci-dessus. Linéarisée colonne par colonne (paternel
// entier, puis maternel entier) — convention confirmée avec l'utilisateur.
const ficheMarquet = `
♂ Pierre Marquet
- Né le 3 février 1830 (mercredi) - Laborie, Saint-Etienne de Maurs, Cantal, Auvergne, France

Parents
- Pierre Marquet 1801-1877
- Marie Brayat 1798-1870

Frères et sœurs
- ♂ Antoine Marquet 1825
- ♂ Cézaire dit Jean Marquet 1827-1898
- ♂ Pierre Marquet 1830
- ♀ Marie Marquet 1832-1893
- ♂ Jean Marquet 1835
- ♀ Agnès Marguerite Marquet 1840-1917

Grands parents paternels, oncles et tantes
- ♂ Maurice Marquet 1762-1823 ⚭ (1787)
- ♀ Marguerite Rouquet 1766-1841
- ♀ Fille Marquet 1787-1787 ✂
- ♀ Anne Marquet 1789- ⊖ (1815)
- ♂ Jean Marquet 1791-1791 ✂
- ♂ Jean Marquet 1792- ✂
- ♀ Catherine Marquet 1796-1864 ✂
- ♂ Antoine Marquès ou Marquet ca 1803- ✂
- ♀ Jeanne Marquet 1804- ✂
- ♂ Etienne Marquet 1805-1806 ✂
- ♀ Marianne Marquet 1809- ✂

Grands parents maternels, oncles et tantes
- ♂ Jean Brayat 1762-1843 ⚭ (1793)
- ♀ Jeanne Puech 1769-1834
- ♂ Pierre Brayat 1796-1864
`

func TestParseFicheMarquet(t *testing.T) {
	f, err := Parse(ficheMarquet)
	if err != nil {
		t.Fatal(err)
	}
	if f.Sujet.Nom != "Pierre Marquet" || f.Sujet.Sexe != "M" {
		t.Fatalf("sujet = %+v", f.Sujet)
	}

	gp := f.GrandsParentsPaternels
	if gp == nil {
		t.Fatal("GrandsParentsPaternels manquant")
	}
	if gp.GrandPere.Nom != "Maurice Marquet" || gp.GrandPere.MariageAnnee != 1787 || !gp.GrandPere.OkMariage {
		t.Fatalf("grand-père paternel = %+v", gp.GrandPere)
	}
	if gp.GrandMere.Nom != "Marguerite Rouquet" || gp.GrandMere.OkMariage {
		t.Fatalf("grand-mère paternelle = %+v", gp.GrandMere)
	}
	if len(gp.Enfants) != 9 {
		t.Fatalf("oncles/tantes paternels = %d, attendu 9 (%+v)", len(gp.Enfants), gp.Enfants)
	}
	if !gp.Enfants[0].SansDescendance {
		t.Fatalf("Fille Marquet (✂) = %+v", gp.Enfants[0])
	}
	anne := gp.Enfants[1]
	if anne.Nom != "Anne Marquet" || anne.SansDescendance || !anne.OkMariage || anne.MariageAnnee != 1815 {
		t.Fatalf("Anne Marquet (⊖ 1815) = %+v", anne)
	}
	antoine := gp.Enfants[5]
	if antoine.Nom != "Antoine Marquès ou Marquet" || !antoine.OkNaissance || antoine.Naissance != 1803 {
		t.Fatalf("Antoine Marquès ou Marquet (ca 1803-) = %+v", antoine)
	}

	gm := f.GrandsParentsMaternels
	if gm == nil {
		t.Fatal("GrandsParentsMaternels manquant")
	}
	if gm.GrandPere.Nom != "Jean Brayat" || gm.GrandPere.MariageAnnee != 1793 {
		t.Fatalf("grand-père maternel = %+v", gm.GrandPere)
	}
	if len(gm.Enfants) != 1 || gm.Enfants[0].Nom != "Pierre Brayat" {
		t.Fatalf("oncles/tantes maternels = %+v", gm.Enfants)
	}
}

func TestConstruireFicheMarquet(t *testing.T) {
	f, err := Parse(ficheMarquet)
	if err != nil {
		t.Fatal(err)
	}
	g := gedcom.Nouveau()
	if _, err := Construire(g, []*Fiche{f}, ""); err != nil {
		t.Fatal(err)
	}

	// Le père du sujet (Pierre Marquet 1801-1877) doit être CHIL de la FAM des
	// grands-parents paternels bien qu'absent de la liste des oncles/tantes.
	pere := add.ChercherHomonymes(g, add.Requete{Nom: "Pierre /Marquet/", Naissance: "1801"})
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

	// "Anne Marquet" (⊖ 1815) : une FAM avec MARR/DATE 1815, conjoint inconnu.
	anne := add.ChercherHomonymes(g, add.Requete{Nom: "Anne /Marquet/", Naissance: "1789"})
	if len(anne) != 1 {
		t.Fatalf("Anne Marquet : %d trouvée(s), attendu 1", len(anne))
	}
	anneRec, _ := g.Get(anne[0].Xref)
	famsAnne := anneRec.Valeurs("FAMS")
	if len(famsAnne) != 1 {
		t.Fatalf("Anne Marquet : %d FAMS, attendu 1", len(famsAnne))
	}
	famAnne, _ := g.Get(famsAnne[0])
	if famAnne.Date("MARR") != "1815" {
		t.Fatalf("mariage d'Anne Marquet : DATE = %q, attendu 1815", famAnne.Date("MARR"))
	}
	if famAnne.Valeur("HUSB") != "" && famAnne.Valeur("WIFE") != "" {
		t.Fatalf("le conjoint d'Anne Marquet ne devrait pas être connu : %+v", famAnne)
	}

	// Une personne marquée ✂ ("Jean Marquet" 1791-1791, homonyme à 1 an de "Jean
	// Marquet" 1792- : add.ChercherHomonymes (fenêtre ±3 ans) trouverait les deux,
	// d'où une recherche par année exacte ici plutôt que via ce helper) porte NCHI 0.
	var jeanRec *gedcom.Record
	for _, ind := range g.Individus() {
		if ind.Nom() != "Jean Marquet" {
			continue
		}
		if an, ok := gedcom.Annee(ind.Date("DEAT")); ok && an == 1791 {
			jeanRec = ind
		}
	}
	if jeanRec == nil {
		t.Fatal("Jean Marquet 1791-1791 introuvable")
	}
	if jeanRec.Valeur("NCHI") != "0" {
		t.Fatalf("Jean Marquet 1791-1791 : NCHI = %q, attendu 0", jeanRec.Valeur("NCHI"))
	}
}

func TestConstruireFichesReelles(t *testing.T) {
	fA, err := Parse(ficheVictoire)
	if err != nil {
		t.Fatal(err)
	}
	fB, err := Parse(ficheFrancois)
	if err != nil {
		t.Fatal(err)
	}
	fC, err := Parse(ficheMarie)
	if err != nil {
		t.Fatal(err)
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

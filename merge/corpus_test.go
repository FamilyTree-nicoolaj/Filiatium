package merge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

// TestFusionCorpusExterne vérifie la fusion de deux exports Gramps réels de deux
// branches d'une même base (sub1_catherine.ged, sub2_marcelRaoul.ged) — ignoré si
// FILIATIUM_CORPUS n'est pas défini ou si ces fichiers n'y sont pas présents. Les
// chiffres attendus ont été mesurés sur le corpus réel avant l'implémentation :
// tout changement les casse volontairement, pour qu'un ajustement de scoring ou
// d'appariement se remarque immédiatement sur un cas concret plutôt qu'un GEDCOM
// artificiel.
func TestFusionCorpusExterne(t *testing.T) {
	corpus := os.Getenv("FILIATIUM_CORPUS")
	if corpus == "" {
		t.Skip("FILIATIUM_CORPUS non défini")
	}
	cheminBase := filepath.Join(corpus, "sub1_catherine.ged")
	cheminApport := filepath.Join(corpus, "sub2_marcelRaoul.ged")
	if _, err := os.Stat(cheminBase); err != nil {
		t.Skipf("%s absent", cheminBase)
	}
	if _, err := os.Stat(cheminApport); err != nil {
		t.Skipf("%s absent", cheminApport)
	}

	base, err := gedcom.Load(cheminBase)
	if err != nil {
		t.Fatal(err)
	}
	apport, err := gedcom.Load(cheminApport)
	if err != nil {
		t.Fatal(err)
	}

	plan := Plan(base, apport, cheminBase, NiveauCertaines)
	nAjouts, nCompletes := 0, 0
	for _, op := range plan.Operations {
		switch op.Op {
		case "add_record":
			nAjouts++
		case "add_lines":
			nCompletes++
		default:
			t.Errorf("opération inattendue dans le plan : %s", op.Op)
		}
	}
	if nAjouts != 27 {
		t.Errorf("add_record = %d, voulu 27", nAjouts)
	}
	if nCompletes != 2 {
		t.Errorf("add_lines = %d, voulu 2", nCompletes)
	}

	dir := t.TempDir()
	cible := filepath.Join(dir, "fusion.ged")
	original, err := os.ReadFile(cheminBase)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cible, original, 0o644); err != nil {
		t.Fatal(err)
	}
	resultat, err := gedcom.Load(cible)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Appliquer(resultat); err != nil {
		t.Fatal(err)
	}

	comptes := map[string]int{}
	for _, r := range resultat.Records {
		comptes[r.Tag]++
	}
	attendu := map[string]int{"INDI": 75, "FAM": 40, "SOUR": 79, "NOTE": 651, "SUBM": 1}
	for tag, n := range attendu {
		if comptes[tag] != n {
			t.Errorf("%s = %d, voulu %d", tag, comptes[tag], n)
		}
	}
}

package gedcom

import "testing"

// TestFusionnerFamilles couvre le cas D1 réel (import "sarraute75") : deux fiches
// distinctes décrivent la même union (conjoint commun, enfant commun), chacune sans
// connaître tout ce que l'autre sait — fam1 ignore l'épouse et un des enfants, fam2
// ignore une NOTE. La fusion doit tout réunir sur fam1, repointer les enfants/conjoint
// vers fam1, et ne laisser aucun pointeur dupliqué derrière elle.
func TestFusionnerFamilles(t *testing.T) {
	g := Nouveau()
	pere, _ := g.AddIndividual("I0001", []string{"1 NAME Jean /Rouquier/", "1 SEX M"}, "")
	mere, _ := g.AddIndividual("I0002", []string{"1 NAME Françoise /Rouquier/", "1 SEX F"}, "")
	commun, _ := g.AddIndividual("I0003", []string{"1 NAME Louise /Rouquier/"}, "")
	uniqueF2, _ := g.AddIndividual("I0004", []string{"1 NAME Pierre /Rouquier/"}, "")

	fam1, _ := g.AddFamily("F0001", pere.Xref, "", nil, "")
	fam1.AjouterLigne("1 CHIL @" + commun.Xref + "@")
	pere.AddFams(fam1.Xref)
	commun.AddFamc(fam1.Xref)

	fam2, _ := g.AddFamily("F0002", pere.Xref, mere.Xref, nil, "")
	fam2.AjouterLigne("1 CHIL @" + commun.Xref + "@")
	fam2.AjouterLigne("1 CHIL @" + uniqueF2.Xref + "@")
	fam2.AjouterLignes(EnligneNote(1, "mariage vers 1760, Saint-Étienne-de-Maurs"))
	pere.AddFams(fam2.Xref)
	mere.AddFams(fam2.Xref)
	commun.AddFamc(fam2.Xref)
	uniqueF2.AddFamc(fam2.Xref)

	fam1Xref, fam2Xref := fam1.Xref, fam2.Xref
	pereXref, mereXref, communXref, uniqueF2Xref := pere.Xref, mere.Xref, commun.Xref, uniqueF2.Xref

	if err := g.FusionnerFamilles(fam1Xref, fam2Xref); err != nil {
		t.Fatal(err)
	}

	// Renumeroter (invoqué par FusionnerFamilles) remplace tous les *Record de g EN
	// PLACE : comme partout ailleurs dans ce dépôt après un appel qui renumérote (voir
	// renumber_test.go), on relit via g.Get plutôt que de réutiliser les pointeurs
	// capturés avant l'appel, qui restent valides en mémoire mais ne reflètent plus
	// l'état du fichier.
	if g.Contains(fam2Xref) {
		t.Error("fam2 aurait dû être supprimée")
	}
	fam1, _ = g.Get(fam1Xref)
	pere, _ = g.Get(pereXref)
	mere, _ = g.Get(mereXref)
	commun, _ = g.Get(communXref)
	uniqueF2, _ = g.Get(uniqueF2Xref)

	if got := fam1.Valeur("WIFE"); got != mere.Xref {
		t.Errorf("WIFE = %q, voulu %q", got, mere.Xref)
	}
	chil := fam1.Valeurs("CHIL")
	if len(chil) != 2 {
		t.Fatalf("CHIL = %v, voulu 2 (commun + unique)", chil)
	}
	if !contient(chil, commun.Xref) || !contient(chil, uniqueF2.Xref) {
		t.Errorf("CHIL = %v, voulu [%s %s]", chil, commun.Xref, uniqueF2.Xref)
	}

	if notes := fam1.Valeurs("NOTE"); len(notes) != 1 || notes[0] != "mariage vers 1760, Saint-Étienne-de-Maurs" {
		t.Errorf("NOTE de fam2 non reprise sur fam1 : %v", notes)
	}

	if got := pere.Valeurs("FAMS"); len(got) != 1 || got[0] != fam1.Xref {
		t.Errorf("père FAMS = %v, voulu [%s] (dédupliqué)", got, fam1.Xref)
	}
	if got := mere.Valeurs("FAMS"); len(got) != 1 || got[0] != fam1.Xref {
		t.Errorf("mère FAMS = %v, voulu [%s]", got, fam1.Xref)
	}
	if got := commun.Valeurs("FAMC"); len(got) != 1 || got[0] != fam1.Xref {
		t.Errorf("enfant commun FAMC = %v, voulu [%s] (dédupliqué)", got, fam1.Xref)
	}
	if got := uniqueF2.Valeurs("FAMC"); len(got) != 1 || got[0] != fam1.Xref {
		t.Errorf("enfant propre à fam2 FAMC = %v, voulu [%s] (repointé)", got, fam1.Xref)
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

func TestFusionnerFamillesErreurs(t *testing.T) {
	g := Nouveau()
	fam, _ := g.AddFamily("F0001", "", "", nil, "")
	if err := g.FusionnerFamilles("F0001", "F0001"); err == nil {
		t.Error("attendu une erreur : fusion avec soi-même")
	}
	if err := g.FusionnerFamilles("F0001", "F9999"); err == nil {
		t.Error("attendu une erreur : F9999 n'existe pas")
	}
	ind, _ := g.AddIndividual("I0001", []string{"1 NAME Test /Test/"}, "")
	if err := g.FusionnerFamilles(fam.Xref, ind.Xref); err == nil {
		t.Error("attendu une erreur : I0001 n'est pas une FAM")
	}
}

// TestFusionnerFamillesConjointDifferent couvre le cas réel (import "sarraute75",
// Jean Rouquier) où D1 rapproche deux FAM sur un conjoint commun (même HUSB) alors que
// l'AUTRE conjoint diffère (deux INDI "Jeanne Lourey" que la déduplication n'a pas
// reconnus comme la même personne) : fusionner quand même aurait fait disparaître le
// lien FAMS de la seconde épouse sans que la FAM fusionnée la porte en retour (règle
// L3). FusionnerFamilles doit refuser plutôt que fusionner à l'aveugle, sans rien
// modifier ni au premier ni au second Gedcom.
func TestFusionnerFamillesConjointDifferent(t *testing.T) {
	g := Nouveau()
	mari, _ := g.AddIndividual("I0001", []string{"1 NAME Jean /Rouquier/"}, "")
	epouseA, _ := g.AddIndividual("I0002", []string{"1 NAME Jeanne /Lourey/"}, "")
	epouseB, _ := g.AddIndividual("I0003", []string{"1 NAME Jeanne /Lourey/"}, "")
	enfant, _ := g.AddIndividual("I0004", []string{"1 NAME Louise /Rouquier/"}, "")

	fam1, _ := g.AddFamily("F0001", mari.Xref, epouseA.Xref, nil, "")
	fam1.AjouterLigne("1 CHIL @" + enfant.Xref + "@")
	fam2, _ := g.AddFamily("F0002", mari.Xref, epouseB.Xref, nil, "")
	fam2.AjouterLigne("1 CHIL @" + enfant.Xref + "@")
	avant1, avant2 := append([]string{}, fam1.Lignes...), append([]string{}, fam2.Lignes...)

	if err := g.FusionnerFamilles(fam1.Xref, fam2.Xref); err == nil {
		t.Fatal("attendu une erreur : WIFE différent (@I0002@ vs @I0003@)")
	}

	if !g.Contains(fam1.Xref) || !g.Contains(fam2.Xref) {
		t.Fatal("les deux FAM devraient toujours exister après un refus")
	}
	fam1After, _ := g.Get(fam1.Xref)
	fam2After, _ := g.Get(fam2.Xref)
	if strJoin(fam1After.Lignes) != strJoin(avant1) || strJoin(fam2After.Lignes) != strJoin(avant2) {
		t.Errorf("un refus ne devrait rien modifier : fam1=%v fam2=%v", fam1After.Lignes, fam2After.Lignes)
	}
}

func strJoin(s []string) string {
	out := ""
	for _, l := range s {
		out += l + "\n"
	}
	return out
}

package patch

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

func TestPreconditionEvenement(t *testing.T) {
	g := charger(t, entete+
		"0 @F0001@ FAM\n1 MARR\n2 DATE 1698\n"+pied)

	if err := (Precondition{Xref: "F0001", Evenement: "MARR", DateVaut: "1698"}).Verifier(g); err != nil {
		t.Errorf("précondition correcte rejetée : %v", err)
	}
	if err := (Precondition{Xref: "F0001", Evenement: "MARR", DateVaut: "1700"}).Verifier(g); err == nil {
		t.Error("précondition fausse acceptée")
	}
}

func TestOperationSetEventDateEtCitation(t *testing.T) {
	g := charger(t, entete+
		"0 @F0111@ FAM\n1 MARR\n2 DATE 1698\n"+
		"0 @S0008@ SOUR\n1 TITL Acte fictif\n"+
		pied)

	c := &Correctif{
		Cible: "peu importe",
		Preconditions: []Precondition{
			{Xref: "F0111", Evenement: "MARR", DateVaut: "1698"},
		},
		Operations: []Operation{
			{Op: "set_event_date", Xref: "F0111", Evenement: "MARR", Valeur: "27 MAY 1700"},
			{Op: "add_citation", Xref: "F0111", Source: "S0008", Evenement: "MARR"},
		},
	}
	if err := c.Appliquer(g); err != nil {
		t.Fatal(err)
	}
	fam, _ := g.Get("F0111")
	if fam.Date("MARR") != "27 MAY 1700" {
		t.Errorf("date = %q", fam.Date("MARR"))
	}

	// Rejouer le même correctif doit maintenant échouer : la précondition ne tient
	// plus (c'est exactement le comportement auto-invalidant des patch_*.py).
	if err := c.Appliquer(g); err == nil {
		t.Error("un correctif déjà appliqué aurait dû être refusé au rejeu")
	}
}

func TestOperationSetLineEtRemoveLine(t *testing.T) {
	g := charger(t, entete+
		"0 @I0001@ INDI\n1 NAME Jean /Untel/\n1 NOTE à corriger\n"+pied)
	c := &Correctif{
		Operations: []Operation{
			{Op: "set_line", Xref: "I0001", Ligne: "1 NAME Jean /Untel/", NouvelleLigne: "1 NAME Jehan /Untel/"},
			{Op: "remove_line", Xref: "I0001", Ligne: "1 NOTE à corriger"},
		},
	}
	if err := c.Appliquer(g); err != nil {
		t.Fatal(err)
	}
	i1, _ := g.Get("I0001")
	if i1.Nom() != "Jehan Untel" {
		t.Errorf("nom = %q", i1.Nom())
	}
	if i1.Valeur("NOTE") != "" {
		t.Errorf("NOTE aurait dû être supprimée")
	}
}

func TestOperationInconnueEtXrefInexistant(t *testing.T) {
	g := charger(t, entete+pied)
	if err := (Operation{Op: "n_importe_quoi", Xref: "I0001"}).Appliquer(g); err == nil {
		t.Error("opération inconnue acceptée")
	}
	if err := (Operation{Op: "set_line", Xref: "I9999"}).Appliquer(g); err == nil {
		t.Error("xref inexistant accepté")
	}
}

func TestChargerRefuseSansOperations(t *testing.T) {
	chemin := filepath.Join(t.TempDir(), "vide.json")
	os.WriteFile(chemin, []byte(`{"cible": "x.ged", "operations": []}`), 0o644)
	if _, err := Charger(chemin); err == nil {
		t.Error("correctif sans opération accepté")
	}
}

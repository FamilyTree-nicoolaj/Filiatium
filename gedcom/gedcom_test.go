package gedcom

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestRoundTrip est le test de sûreté le plus important du paquet : charger puis
// réécrire sans aucune modification doit reproduire le fichier à l'octet près. C'est
// ce qui garantit que Save() ne "corrige" jamais en silence une particularité du
// fichier source (BOM, fin de ligne, valeur vide...).
func TestRoundTrip(t *testing.T) {
	original, err := os.ReadFile("testdata/mini.ged")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	copie := filepath.Join(dir, "mini.ged")
	if err := os.WriteFile(copie, original, 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := Load(copie)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Save(""); err != nil {
		t.Fatal(err)
	}
	relu, err := os.ReadFile(copie)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, relu) {
		t.Errorf("round-trip non identique :\n--- original ---\n%s\n--- relu ---\n%s", original, relu)
	}
}

func TestRoundTripBOMEtCRLF(t *testing.T) {
	contenu := "0 HEAD\r\n1 CHAR UTF-8\r\n0 TRLR\r\n"
	original := append(append([]byte{}, bomUTF8...), []byte(contenu)...)
	dir := t.TempDir()
	chemin := filepath.Join(dir, "crlf.ged")
	if err := os.WriteFile(chemin, original, 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := Load(chemin)
	if err != nil {
		t.Fatal(err)
	}
	if !g.bom || !g.crlf {
		t.Errorf("BOM/CRLF non détectés : bom=%v crlf=%v", g.bom, g.crlf)
	}
	if _, err := g.Save(""); err != nil {
		t.Fatal(err)
	}
	relu, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, relu) {
		t.Errorf("round-trip BOM/CRLF non identique : %q != %q", relu, original)
	}
}

func TestLoadLigneMalformee(t *testing.T) {
	dir := t.TempDir()
	chemin := filepath.Join(dir, "mauvais.ged")
	os.WriteFile(chemin, []byte("ceci n'est pas du gedcom\n0 TRLR\n"), 0o644)
	if _, err := Load(chemin); err == nil {
		t.Error("attendu une erreur sur une ligne hors enregistrement")
	}
}

func TestSaveConflitConcurrent(t *testing.T) {
	dir := t.TempDir()
	chemin := filepath.Join(dir, "conflit.ged")
	os.WriteFile(chemin, []byte("0 HEAD\n0 TRLR\n"), 0o644)
	g, err := Load(chemin)
	if err != nil {
		t.Fatal(err)
	}
	// une autre "session" écrit entretemps
	if err := os.WriteFile(chemin, []byte("0 HEAD\n1 NOTE modifié ailleurs\n0 TRLR\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = g.Save("")
	if err == nil {
		t.Fatal("attendu ErrConflitConcurrent")
	}
	if _, ok := err.(*ErrConflitConcurrent); !ok {
		t.Errorf("erreur inattendue : %T %v", err, err)
	}
}

func chargerMini(t *testing.T) *Gedcom {
	t.Helper()
	dir := t.TempDir()
	copie := filepath.Join(dir, "mini.ged")
	original, err := os.ReadFile("testdata/mini.ged")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copie, original, 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := Load(copie)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestAccesseurs(t *testing.T) {
	g := chargerMini(t)
	jean, ok := g.Get("I0001")
	if !ok {
		t.Fatal("I0001 introuvable")
	}
	if jean.Nom() != "Jean Untel" {
		t.Errorf("Nom() = %q, voulu %q", jean.Nom(), "Jean Untel")
	}
	if jean.Patronyme() != "Untel" {
		t.Errorf("Patronyme() = %q, voulu %q", jean.Patronyme(), "Untel")
	}
	if got := jean.Date("BIRT"); got != "1 JAN 1800" {
		t.Errorf("Date(BIRT) = %q", got)
	}
	if got := jean.Valeur("FAMS"); got != "F0001" {
		t.Errorf("Valeur(FAMS) = %q", got)
	}

	paul, ok := g.Get("@I0003@") // avec les @, comme dans les pointeurs bruts
	if !ok {
		t.Fatal("I0003 introuvable")
	}
	fp := paul.FamcPedi()
	if len(fp) != 1 || fp[0].Fam != "F0001" || fp[0].Pedi != "birth" {
		t.Errorf("FamcPedi() = %+v", fp)
	}

	if len(g.Individus()) != 3 {
		t.Errorf("Individus() = %d, voulu 3", len(g.Individus()))
	}
	if len(g.Familles()) != 1 {
		t.Errorf("Familles() = %d, voulu 1", len(g.Familles()))
	}
}

func TestArbreGenealogique(t *testing.T) {
	g := chargerMini(t)
	parents, err := g.Parents("I0003", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(parents) != 1 || parents[0][0] != "I0001" || parents[0][1] != "I0002" {
		t.Errorf("Parents(I0003) = %v", parents)
	}

	enfants, err := g.Enfants("I0001")
	if err != nil {
		t.Fatal(err)
	}
	if len(enfants) != 1 || enfants[0] != "I0003" {
		t.Errorf("Enfants(I0001) = %v", enfants)
	}

	conjoints, err := g.Conjoints("I0001")
	if err != nil {
		t.Fatal(err)
	}
	if len(conjoints) != 1 || conjoints[0].Xref != "I0002" || conjoints[0].Fam != "F0001" {
		t.Errorf("Conjoints(I0001) = %v", conjoints)
	}

	sosa := g.Sosa("I0003")
	if sosa["I0003"] != 1 || sosa["I0001"] != 2 || sosa["I0002"] != 3 {
		t.Errorf("Sosa(I0003) = %v", sosa)
	}
}

func TestAjoutsEtProchainXref(t *testing.T) {
	g := chargerMini(t)

	if got := g.ProchainXref("I"); got != "I0004" {
		t.Errorf("ProchainXref(I) = %q, voulu I0004", got)
	}

	sour, err := g.AddSource("S0002", "Registre paroissial fictif", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !g.Contains("S0002") || sour.Tag != "SOUR" {
		t.Errorf("AddSource n'a pas créé S0002 correctement")
	}
	if _, err := g.AddSource("S0002", "doublon", "", "", "", ""); err == nil {
		t.Error("attendu une erreur sur un xref déjà pris")
	}

	indi, err := g.AddIndividual("I0004", []string{"1 NAME Alice /Untel/", "1 SEX F"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if indi.Nom() != "Alice Untel" {
		t.Errorf("AddIndividual : nom = %q", indi.Nom())
	}

	fam, err := g.AddFamily("F0002", "I0001", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if fam.Valeur("HUSB") != "I0001" {
		t.Errorf("AddFamily : HUSB = %q", fam.Valeur("HUSB"))
	}

	// TRLR doit rester le dernier enregistrement après tous ces ajouts.
	dernier := g.Records[len(g.Records)-1]
	if dernier.Tag != "TRLR" {
		t.Errorf("TRLR n'est plus en dernière position : %s", dernier.Tag)
	}
}

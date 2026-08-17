package gedcom

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestRoundTripCorpusExterne vérifie qu'un Load+Save sans modification ne change pas
// un octet sur le corpus généalogique réel. Ce corpus contient des données
// personnelles et n'est jamais copié dans ce dépôt (voir .gitignore) : le test est
// ignoré si FILIATIUM_CORPUS n'est pas défini. scripts/roundtrip.sh (et `make
// roundtrip`) le positionnent sur le dossier réel.
func TestRoundTripCorpusExterne(t *testing.T) {
	corpus := os.Getenv("FILIATIUM_CORPUS")
	if corpus == "" {
		t.Skip("FILIATIUM_CORPUS non défini — voir scripts/roundtrip.sh")
	}
	var fichiers []string
	err := filepath.WalkDir(corpus, func(chemin string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(chemin) == ".ged" {
			fichiers = append(fichiers, chemin)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fichiers) == 0 {
		t.Fatalf("aucun .ged trouvé sous %s", corpus)
	}
	for _, f := range fichiers {
		f := f
		t.Run(filepath.Base(f), func(t *testing.T) {
			original, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			copie := filepath.Join(dir, filepath.Base(f))
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
				t.Errorf("round-trip non identique pour %s (%d octets -> %d octets)",
					filepath.Base(f), len(original), len(relu))
			}
		})
	}
}

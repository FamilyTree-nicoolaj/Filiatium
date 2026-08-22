package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/geneanet"
	"github.com/FamilyTree-nicoolaj/filiatium/rules"
)

const aideImport = `
filiatium import <dst.ged> <fiche1.png> [fiche2.png ...] [options]

Construit un arbre GEDCOM complet à partir de captures d'écran de fiches
individuelles Geneanet (parents, union(s)/enfants, frères et sœurs, demi-frères et
demi-sœurs, profession, sources), en dédupliquant automatiquement les personnes qui
apparaissent sur plusieurs fiches (même patronyme et prénom, et — quand elle est
connue des deux côtés — même année de naissance).

L'OCR est fait en interne via le binaire système "tesseract" (jamais invoqué à la
main) : erreur claire et actionnable si le binaire est absent du PATH. --texte évite
cet appel quand les fichiers sont déjà du texte (OCR fait à part, ou copié-collé).

Écrit toujours vers un nouveau fichier dst.ged, jamais dans un fichier déjà existant.
Rejoue le registre de règles ("filiatium check") avant/après la construction ; refuse
d'écrire si elle introduit un signalement structure/liens/doublons (un vrai défaut).
Un signalement de réalisme (ex. R13, aucun décès enregistré) reste affiché dans le
rapport sans bloquer l'écriture : sur un arbre neuf, comparé à un fichier vide, c'est
un retour légitime sur des données réellement incomplètes, pas un défaut introduit
par la construction.

Options :
  --texte     les fichiers d'entrée sont déjà du texte — tesseract n'est pas invoqué
  --auteur    utilisatrice/contributrice Geneanet source de la capture (ex.
              "Sylvie DUJARDIN (sylvied58)"), attribuée à la source Geneanet créée
              pour cet import
  --write     écrire dst.ged (sinon simulation : rapport seul)
  --json      rapport en JSON plutôt qu'en texte

Exemple :
  filiatium import arbre.ged fiche*.png --auteur "Sylvie DUJARDIN (sylvied58)" --write
`

func init() {
	commandes = append(commandes, Commande{
		Nom:           "import",
		Description:   "Construire un GEDCOM depuis des captures Geneanet (OCR interne)",
		AideDetaillee: aideImport,
		Executer:      cmdImport,
	})
}

// flagsImport enregistre les options de `import` sur fs — voir flagsCheck
// (cmd_check.go) pour pourquoi ceci est factorisé à part.
func flagsImport(fs *flag.FlagSet) (texte *bool, auteur *string, ecrire, sortieJSON *bool) {
	texte = fs.Bool("texte", false, "fichiers déjà en texte — pas d'appel à tesseract")
	auteur = fs.String("auteur", "", `utilisatrice/contributrice Geneanet source de la capture, ex. "Sylvie DUJARDIN (sylvied58)"`)
	ecrire = fs.Bool("write", false, "écrire dst.ged (sinon simulation)")
	sortieJSON = fs.Bool("json", false, "rapport en JSON plutôt qu'en texte")
	return
}

func cmdImport(argv []string) int {
	if aideSiDemandee("import", argv) {
		return 0
	}
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	texte, auteur, ecrire, sortieJSON := flagsImport(fs)
	fs.Parse(argvPourFlagSet(fs, argv))

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage : filiatium import <dst.ged> <fiche1.png> [fiche2.png ...] [options]")
		return 2
	}
	cheminDst, sources := fs.Arg(0), fs.Args()[1:]
	for _, src := range sources {
		if memeFichier(cheminDst, src) {
			fmt.Fprintln(os.Stderr, "erreur : dst.ged doit être différent des fiches d'entrée")
			return 2
		}
	}

	if !*texte {
		if err := geneanet.TesseractDisponible(); err != nil {
			fmt.Fprintln(os.Stderr, "erreur :", err)
			return 2
		}
	}

	var fiches []*geneanet.Fiche
	for _, src := range sources {
		brut, err := lireOuOCR(src, *texte)
		if err != nil {
			fmt.Fprintln(os.Stderr, "erreur :", err)
			return 2
		}
		f, err := geneanet.Parse(brut)
		if err != nil {
			fmt.Fprintf(os.Stderr, "erreur : %s : %v\n", src, err)
			return 2
		}
		fiches = append(fiches, f)
	}

	g := gedcom.Nouveau()
	seuils, err := config.Charger(cheminDst)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	avant := executerRegles(g, rules.Registre, seuils)

	rapport, err := geneanet.Construire(g, fiches, *auteur)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

	apres := executerRegles(g, rules.Registre, seuils)
	// La garde d'écriture compare ici à un arbre vide (gedcom.Nouveau) plutôt qu'à un
	// arbre existant : un signalement de réalisme (ex. R13, "aucun décès enregistré")
	// est alors quasi certain sur n'importe quelle capture réelle (beaucoup
	// d'ascendants n'ont simplement pas de date de décès connue) — un retour légitime
	// sur les données, pas un défaut introduit par la construction. Seules les
	// catégories structure/liens/doublons (des défauts réels : lien cassé, doublon
	// structurel) bloquent --write ; le réalisme reste affiché dans le rapport.
	nouveaux := signalementsNouveaux(sansRealisme(avant), sansRealisme(apres))

	if *sortieJSON {
		afficherImportJSON(cheminDst, sources, rapport, apres, *ecrire && len(nouveaux) == 0)
	} else {
		afficherImportTexte(cheminDst, sources, rapport, apres)
	}

	if len(nouveaux) > 0 {
		if !*sortieJSON {
			fmt.Fprintln(os.Stderr, "\n⚠ écriture annulée : cette construction introduit de nouveaux signalements (voir \"filiatium check\")")
			for _, n := range nouveaux {
				fmt.Fprintln(os.Stderr, "    "+n)
			}
		}
		return 1
	}

	if !*ecrire {
		if !*sortieJSON {
			fmt.Println("\n(simulation — relancer avec --write pour écrire dst.ged)")
		}
		return 0
	}

	if _, err := g.Save(cheminDst); err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	if !*sortieJSON {
		fmt.Println("\nécrit :", cheminDst)
	}
	return 0
}

// sansRealisme retire les signalements de catégorie "realisme" d'un résultat de
// executerRegles — voir le commentaire sur nouveaux dans cmdImport.
func sansRealisme(m map[string][]rules.Finding) map[string][]rules.Finding {
	out := make(map[string][]rules.Finding, len(m))
	for _, r := range rules.Registre {
		if r.Categorie == "realisme" {
			continue
		}
		if findings, ok := m[r.ID]; ok {
			out[r.ID] = findings
		}
	}
	return out
}

// lireOuOCR lit le texte d'une fiche : directement si texte==true, sinon via
// geneanet.OCR (tesseract).
func lireOuOCR(chemin string, texte bool) (string, error) {
	if texte {
		octets, err := os.ReadFile(chemin)
		if err != nil {
			return "", err
		}
		return string(octets), nil
	}
	return geneanet.OCR(chemin)
}

func afficherImportTexte(cheminDst string, sources []string, rapport *geneanet.Rapport, apres map[string][]rules.Finding) {
	fmt.Printf("dst     : %s\nfiches  : %d\n\n", cheminDst, len(sources))
	fmt.Printf("individus créés : %d\nfamilles créées  : %d\nsources créées   : %d\n", rapport.Individus, rapport.Familles, rapport.Sources)
	for _, a := range rapport.Ambigus {
		fmt.Println("  ⚠", a)
	}
	total := 0
	for _, findings := range apres {
		total += len(findings)
	}
	fmt.Printf("\nfiltre \"check\" après construction : %d signalement(s)\n", total)
}

func afficherImportJSON(cheminDst string, sources []string, rapport *geneanet.Rapport, apres map[string][]rules.Finding, ecrit bool) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{
		"dst": cheminDst, "fiches": sources, "rapport": rapport, "check_apres": apres, "ecrit": ecrit,
	})
}

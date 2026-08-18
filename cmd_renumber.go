package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/renumber"
)

const aideRenumber = `
filiatium renumber <fichier.ged> (--source <xref> | --decalage <n> | --prefixe <lettre>) [options]
filiatium renumber --depuis-table <table.json> [options]

Renumérote tous les xref INDI et FAM d'un GEDCOM (jamais SOUR/NOTE/OBJE/SUBM/
REPO, qui gardent les leurs), selon une des trois stratégies ci-dessous, puis
rejoue optionnellement la correspondance obtenue sur des fichiers .md de
recherche qui la citent (mode --depuis-table, étape séparée et explicite).

Trois stratégies, une seule à la fois :
  --source <xref>    numérotation cohérente en repartant de cet individu :
                      parcours en largeur (conjoints, enfants, parents, toutes
                      pédigrées FAMC) ; toute composante déconnectée est
                      ensuite balayée à son tour, en ordre fichier.
  --decalage <n>      décale de n le numéro de chaque xref existant, ex.
                      "I0001" -> "I5001" avec --decalage 5000 — garde la
                      lettre de tag et au moins la largeur d'origine.
  --prefixe <lettre>  ajoute lettre devant chaque xref existant tel quel, ex.
                      "I0001" -> "ZI0001" avec --prefixe Z.

--decalage/--prefixe sont utiles pour namespacer un arbre secondaire avant de
l'analyser avec "merge --analyse", plutôt que de laisser merge choisir au cas
par cas sur collision réelle.

Options (mode --source/--decalage/--prefixe) :
  --table <fichier>  écrire la table de correspondance ancien->nouveau xref
                      (JSON), indépendamment de --write — à relire avant
                      d'écrire, et à passer ensuite à --depuis-table
  --write            écrire le résultat dans le GEDCOM (sinon simulation)
  --json             rapport en JSON plutôt qu'en texte

Options (mode --depuis-table, mise à jour des notes) :
  --depuis-table <table.json>  table de correspondance déjà produite
  --notes <dossier>            dossier des .md à mettre à jour (défaut :
                                celui du .ged enregistré dans la table)
  --write                      écrire les fichiers modifiés (sinon simulation)
  --json                       rapport en JSON plutôt qu'en texte

Dans chaque .md de <dossier> (motif "*.md", non récursif), chaque occurrence
en MOT ENTIER d'un ancien xref de la table (ex. "I0517", "[I0517]", ou cité
"@F0271@" dans une ligne GEDCOM reproduite) est remplacée par le nouveau —
jamais un motif générique, seulement les xref réellement issus de cette
renumérotation précise.

Une renumérotation est une relabelisation bijective pure : elle ne peut
introduire aucun signalement nouveau (voir "filiatium check"), donc renumber
ne rejoue jamais le registre de règles avant d'écrire, contrairement à
fix/add/apply.

Exemples :
  filiatium renumber family.ged --source I0001 --table renum.json
  filiatium renumber family.ged --source I0001 --table renum.json --write
  filiatium renumber secondary.ged --decalage 5000 --write
  filiatium renumber --depuis-table renum.json
  filiatium renumber --depuis-table renum.json --notes ~/Documents/Genealogie --write
`

func init() {
	commandes = append(commandes, Commande{
		Nom:           "renumber",
		Description:   "Renumérotation complète des INDI/FAM (--source, --decalage ou --prefixe) et mise à jour des notes .md",
		AideDetaillee: aideRenumber,
		Executer:      cmdRenumber,
	})
}

// flagsRenumber enregistre les options de `renumber` sur fs — voir flagsCheck
// (cmd_check.go) pour pourquoi ceci est factorisé à part.
func flagsRenumber(fs *flag.FlagSet) (source, decalage, prefixe, sortieTable, depuisTable, notes *string, ecrire, sortieJSON *bool) {
	source = fs.String("source", "", "individu source (xref) : point de départ du parcours en largeur")
	decalage = fs.String("decalage", "", "décale de n le numéro de chaque xref INDI/FAM existant (ex. 5000)")
	prefixe = fs.String("prefixe", "", "ajoute cette lettre devant chaque xref INDI/FAM existant (ex. Z)")
	sortieTable = fs.String("table", "", "écrire la table de correspondance ancien->nouveau xref (JSON)")
	depuisTable = fs.String("depuis-table", "", "mode notes : rejouer une table déjà produite sur les fichiers .md")
	notes = fs.String("notes", "", "dossier des .md à mettre à jour (mode --depuis-table ; défaut : celui du .ged enregistré dans la table)")
	ecrire = fs.Bool("write", false, "écrire le résultat (sinon simulation)")
	sortieJSON = fs.Bool("json", false, "sortie JSON plutôt que texte (pour un usage scripté/agent)")
	return
}

func cmdRenumber(argv []string) int {
	if aideSiDemandee("renumber", argv) {
		return 0
	}
	fs := flag.NewFlagSet("renumber", flag.ExitOnError)
	source, decalage, prefixe, sortieTable, depuisTable, notes, ecrire, sortieJSON := flagsRenumber(fs)
	fs.Parse(argvPourFlagSet(fs, argv))

	strategies := 0
	for _, s := range []string{*source, *decalage, *prefixe} {
		if s != "" {
			strategies++
		}
	}
	switch {
	case strategies > 1:
		fmt.Fprintln(os.Stderr, "usage : une seule stratégie à la fois (--source, --decalage ou --prefixe)")
		return 2
	case strategies == 1 && *depuisTable != "":
		fmt.Fprintln(os.Stderr, "usage : --depuis-table ne se combine pas avec --source/--decalage/--prefixe")
		return 2
	case strategies == 1:
		return cmdRenumberGed(fs, *source, *decalage, *prefixe, *sortieTable, *ecrire, *sortieJSON)
	case *depuisTable != "":
		return cmdRenumberNotes(*depuisTable, *notes, *ecrire, *sortieJSON)
	default:
		fmt.Fprintln(os.Stderr, "usage : filiatium renumber <fichier.ged> (--source <xref> | --decalage <n> | --prefixe <lettre>) [options]")
		fmt.Fprintln(os.Stderr, "    ou : filiatium renumber --depuis-table <table.json> [options]")
		return 2
	}
}

// cmdRenumberGed calcule (et, avec --write, applique) la renumérotation du GEDCOM
// positionnel selon la stratégie sélectionnée par le flag non vide parmi
// source/decalage/prefixe (un seul l'est, garanti par cmdRenumber).
func cmdRenumberGed(fs *flag.FlagSet, source, decalage, prefixe, sortieTable string, ecrire, sortieJSON bool) int {
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage : filiatium renumber <fichier.ged> (--source <xref> | --decalage <n> | --prefixe <lettre>) [options]")
		return 2
	}
	chemin := fs.Arg(0)
	g, err := gedcom.Load(chemin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

	var strategie, parametre string
	var table map[string]string
	switch {
	case source != "":
		strategie, parametre = "source", source
		table, err = renumber.Calculer(g, source)
	case decalage != "":
		n, errConv := strconv.Atoi(decalage)
		if errConv != nil {
			fmt.Fprintln(os.Stderr, "erreur : --decalage doit être un entier, obtenu", decalage)
			return 2
		}
		strategie, parametre = "decalage", decalage
		table, err = renumber.CalculerDecalage(g, n)
	default:
		strategie, parametre = "prefixe", prefixe
		table, err = renumber.CalculerPrefixe(g, prefixe)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

	if sortieTable != "" {
		t := renumber.Table{Cible: chemin, Strategie: strategie, Parametre: parametre, Correspondance: table}
		octets, err := json.MarshalIndent(t, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "erreur :", err)
			return 2
		}
		if err := os.WriteFile(sortieTable, octets, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "erreur :", err)
			return 2
		}
		if !sortieJSON {
			fmt.Printf("table de correspondance écrite : %s (%d xref)\n", sortieTable, len(table))
		}
	}

	if ecrire {
		g.Renumeroter(table)
		if _, err := g.Save(""); err != nil {
			fmt.Fprintln(os.Stderr, "erreur :", err)
			return 2
		}
	}

	afficherRenumberGed(chemin, strategie, parametre, table, ecrire, sortieJSON)
	return 0
}

func afficherRenumberGed(chemin, strategie, parametre string, table map[string]string, ecrit, sortieJSON bool) {
	if sortieJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{
			"fichier": chemin, "strategie": strategie, "parametre": parametre,
			"xrefs_renumerotes": len(table), "ecrit": ecrit,
		})
		return
	}
	fmt.Printf("%s : %d xref renuméroté(s) (stratégie %s, paramètre %q)\n", chemin, len(table), strategie, parametre)
	if ecrit {
		fmt.Println("écrit.")
	} else {
		fmt.Println("(simulation — relancer avec --write pour écrire)")
	}
}

// cmdRenumberNotes rejoue la table de correspondance lue dans cheminTable sur les
// *.md du dossier choisi (dossierFlag si fourni, sinon celui de la "cible"
// enregistrée dans la table) — jamais embarqué dans cmdRenumberGed --write, mode
// séparé et explicite (voir aideRenumber).
func cmdRenumberNotes(cheminTable, dossierFlag string, ecrire, sortieJSON bool) int {
	octets, err := os.ReadFile(cheminTable)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	var t renumber.Table
	if err := json.Unmarshal(octets, &t); err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", cheminTable, err)
		return 2
	}

	dossier := dossierFlag
	if dossier == "" {
		if t.Cible == "" {
			fmt.Fprintln(os.Stderr, `erreur : --notes non fourni et la table n'indique pas de "cible" pour en déduire un dossier`)
			return 2
		}
		dossier = filepath.Dir(resoudreCible(cheminTable, t.Cible))
	}
	if _, err := os.Stat(dossier); err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

	fichiers, err := filepath.Glob(filepath.Join(dossier, "*.md"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

	type resultatFichier struct {
		Fichier       string `json:"fichier"`
		Remplacements int    `json:"remplacements"`
	}
	var resultats []resultatFichier
	total := 0
	for _, f := range fichiers {
		contenu, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "erreur :", err)
			return 2
		}
		nouveau, n := renumber.AppliquerNotes(string(contenu), t.Correspondance)
		if n == 0 {
			continue
		}
		resultats = append(resultats, resultatFichier{Fichier: f, Remplacements: n})
		total += n
		if ecrire {
			if err := os.WriteFile(f, []byte(nouveau), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, "erreur :", err)
				return 2
			}
		}
	}

	if sortieJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{
			"dossier": dossier, "fichiers": resultats, "remplacements_total": total, "ecrit": ecrire,
		})
		return 0
	}
	fmt.Printf("dossier : %s\n", dossier)
	if len(resultats) == 0 {
		fmt.Println("aucun remplacement (aucun xref de la table cité dans les .md de ce dossier)")
		return 0
	}
	for _, r := range resultats {
		fmt.Printf("    %s : %d remplacement(s)\n", r.Fichier, r.Remplacements)
	}
	fmt.Printf("%d remplacement(s) au total dans %d fichier(s)\n", total, len(resultats))
	if ecrire {
		fmt.Println("écrit.")
	} else {
		fmt.Println("(simulation — relancer avec --write pour écrire)")
	}
	return 0
}

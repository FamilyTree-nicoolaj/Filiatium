package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/FamilyTree-nicoolaj/filiatium/add"
	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/rules"
)

const aideAdd = `
filiatium add <fichier.ged> [options]

Ajoute un individu en câblant systématiquement les deux sens de chaque lien de
parenté (FAM.CHIL + INDI.FAMC ; FAM.HUSB/WIFE + INDI.FAMS), y compris côté
parent quand une nouvelle famille est créée. Recherche d'homonyme avant
création (refuse par défaut si un candidat existe). Rejoue tout le registre de
règles avant d'écrire ; refuse si l'ajout introduit un signalement nouveau.

Options :
  --nom "Prénom /Patronyme/"   nom complet au format GEDCOM, ex. "Jean /Dupret/"
  --sexe M|F                   M, F, ou vide si inconnu
  --naiss "12 MAR 1805"        date de naissance GEDCOM
  --deces "..."                date de décès GEDCOM
  --pere <xref>                xref du père
  --mere <xref>                xref de la mère
  --conjoint <xref>            xref du conjoint
  --note "..."                 justification, ajoutée en 1 NOTE
  --fichier lot.json           fichier JSON décrivant un ou plusieurs ajouts en lot
  --force                      ajouter même si un homonyme potentiel existe
  --write                      écrire le résultat (sinon simulation)
  --json                       sortie JSON plutôt que texte (pour un usage scripté/agent)

Sans --nom ni --fichier, et si l'entrée standard est un terminal, lance un
assistant interactif qui pose les mêmes questions une à une.

Exemples :
  filiatium add family.ged --nom "Jean /Dupret/" --sexe M --naiss "12 MAR 1805" \
    --pere I0123 --mere I0124 --conjoint I0200 --write
  filiatium add family.ged --fichier lot.json --write
`

func init() {
	commandes = append(commandes, Commande{
		Nom:           "add",
		Description:   "Ajouter un individu en câblant tous ses liens de parenté (--nom, --fichier, ou assistant)",
		AideDetaillee: aideAdd,
		Executer:      cmdAdd,
	})
}

func cmdAdd(argv []string) int {
	if aideSiDemandee("add", argv) {
		return 0
	}
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	nomF := fs.String("nom", "", `nom complet au format GEDCOM, ex. "Jean /Dupret/"`)
	sexeF := fs.String("sexe", "", "M, F, ou vide si inconnu")
	naissF := fs.String("naiss", "", `date de naissance GEDCOM, ex. "12 MAR 1805"`)
	decesF := fs.String("deces", "", "date de décès GEDCOM")
	pereF := fs.String("pere", "", "xref du père")
	mereF := fs.String("mere", "", "xref de la mère")
	conjointF := fs.String("conjoint", "", "xref du conjoint")
	noteF := fs.String("note", "", "justification, ajoutée en 1 NOTE")
	fichierF := fs.String("fichier", "", "fichier JSON décrivant un ou plusieurs ajouts en lot")
	force := fs.Bool("force", false, "ajouter même si un homonyme potentiel existe")
	ecrire := fs.Bool("write", false, "écrire le résultat (sinon simulation)")
	sortieJSON := fs.Bool("json", false, "sortie JSON plutôt que texte (pour un usage scripté/agent)")
	fs.Parse(argvPourFlagSet(fs, argv))

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage : filiatium add <fichier.ged> [--nom ... | --fichier lot.json] [--write] [--force] [--json]")
		return 2
	}
	chemin := fs.Arg(0)

	g, err := gedcom.Load(chemin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	seuils, err := config.Charger(chemin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	avant := executerRegles(g, rules.Registre, seuils)

	var requetes []add.Requete
	switch {
	case *fichierF != "":
		requetes, err = chargerRequetesJSON(*fichierF)
		if err != nil {
			fmt.Fprintln(os.Stderr, "erreur :", err)
			return 2
		}
	case *nomF != "":
		requetes = []add.Requete{{
			Nom: *nomF, Sexe: *sexeF, Naissance: *naissF, Deces: *decesF,
			Pere: *pereF, Mere: *mereF, Conjoint: *conjointF, Note: *noteF,
			IgnorerHomonymes: *force,
		}}
	default:
		if !terminalInteractif() {
			fmt.Fprintln(os.Stderr, "aucun --nom ni --fichier fourni, et l'entrée n'est pas un terminal")
			return 2
		}
		req, ok := assistantAjout()
		if !ok {
			return 2
		}
		req.IgnorerHomonymes = *force
		requetes = []add.Requete{req}
	}

	var resultats []*add.Resultat
	for _, req := range requetes {
		res, err := add.Ajouter(g, req)
		if err != nil {
			if *sortieJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.Encode(map[string]any{"erreur": err.Error()})
			} else {
				fmt.Fprintln(os.Stderr, "⚠", err)
			}
			return 1
		}
		resultats = append(resultats, res)
	}

	if !*sortieJSON {
		for _, res := range resultats {
			fmt.Printf("%s :\n", res.Xref)
			for _, l := range res.Diff {
				fmt.Println("  " + l)
			}
			if len(res.Homonymes) > 0 {
				fmt.Println("  (ajouté malgré des homonymes potentiels — voir --force)")
			}
		}
	}

	if !*ecrire {
		return finAdd(*sortieJSON, resultats, false, nil)
	}

	// Auto-vérification : rejouer tout le registre sur le résultat en mémoire et
	// refuser d'écrire si l'ajout introduit un signalement nouveau — c'est ce qui
	// rend l'ajout non équivoque, pas seulement le câblage des deux sens.
	apres := executerRegles(g, rules.Registre, seuils)
	nouveaux := signalementsNouveaux(avant, apres)
	if len(nouveaux) > 0 {
		if !*sortieJSON {
			fmt.Fprintln(os.Stderr, "\n⚠ écriture annulée : cet ajout introduit de nouveaux signalements :")
			for _, n := range nouveaux {
				fmt.Fprintln(os.Stderr, "    "+n)
			}
		}
		return finAdd(*sortieJSON, resultats, false, nouveaux)
	}

	if _, err := g.Save(""); err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	return finAdd(*sortieJSON, resultats, true, nil)
}

// finAdd centralise la sortie finale de `add`, texte ou JSON selon --json.
func finAdd(sortieJSON bool, resultats []*add.Resultat, ecrit bool, nouveaux []string) int {
	if sortieJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{
			"resultats": resultats, "ecrit": ecrit, "nouveaux_signalements": nouveaux,
		})
	} else if !ecrit && len(nouveaux) == 0 {
		fmt.Println("\n(simulation — relancer avec --write pour écrire)")
	} else if ecrit {
		fmt.Println("écrit.")
	}
	if len(nouveaux) > 0 {
		return 1
	}
	return 0
}

func chargerRequetesJSON(chemin string) ([]add.Requete, error) {
	octets, err := os.ReadFile(chemin)
	if err != nil {
		return nil, err
	}
	var un add.Requete
	if err := json.Unmarshal(octets, &un); err == nil && un.Nom != "" {
		return []add.Requete{un}, nil
	}
	var plusieurs []add.Requete
	if err := json.Unmarshal(octets, &plusieurs); err != nil {
		return nil, fmt.Errorf("%s : attendu un objet ou un tableau d'ajouts : %w", chemin, err)
	}
	return plusieurs, nil
}

func assistantAjout() (add.Requete, bool) {
	lecteur := bufio.NewScanner(os.Stdin)
	demander := func(invite string) (string, bool) {
		fmt.Print(invite)
		if !lecteur.Scan() {
			return "", false
		}
		return strings.TrimSpace(lecteur.Text()), true
	}
	nom, ok := demander(`nom complet (ex. "Jean /Dupret/") : `)
	if !ok || nom == "" {
		return add.Requete{}, false
	}
	sexe, _ := demander("sexe (M/F, vide si inconnu) : ")
	naiss, _ := demander("date de naissance (vide si inconnue) : ")
	deces, _ := demander("date de décès (vide si inconnue) : ")
	pere, _ := demander("xref du père (vide si aucun) : ")
	mere, _ := demander("xref de la mère (vide si aucune) : ")
	conjoint, _ := demander("xref du conjoint (vide si aucun) : ")
	note, _ := demander("justification (vide si aucune) : ")
	return add.Requete{
		Nom: nom, Sexe: sexe, Naissance: naiss, Deces: deces,
		Pere: pere, Mere: mere, Conjoint: conjoint, Note: note,
	}, true
}

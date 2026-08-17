package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/fix"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/rules"
)

const aideFix = `
filiatium fix <fichier.ged> [options]

Corrige ce qui est mécaniquement sûr : lien réciproque manquant (L1/L2),
pointeur dupliqué (D3/D4), ligne de plus de 255 caractères repliée en CONC (S3).
Rien d'autre n'est jamais corrigé automatiquement — voir "filiatium check help"
pour le détail de chaque règle.

Options :
  --write         appliquer les corrections retenues (sinon simulation)
  --interactif    confirmer chaque correction individuellement
  --json          sortie JSON plutôt que texte (pour un usage scripté/agent)

Après --write, tout le registre de règles est rejoué automatiquement ;
l'écriture est annulée si une correction introduit un signalement nouveau.

Exemples :
  filiatium fix family.ged
  filiatium fix family.ged --write
  filiatium fix family.ged --interactif
`

func init() {
	commandes = append(commandes, Commande{
		Nom:           "fix",
		Description:   "Corriger ce qui est mécaniquement sûr (liens réciproques, doublons, lignes longues)",
		AideDetaillee: aideFix,
		Executer:      cmdFix,
	})
}

func cmdFix(argv []string) int {
	if aideSiDemandee("fix", argv) {
		return 0
	}
	fs := flag.NewFlagSet("fix", flag.ExitOnError)
	ecrire := fs.Bool("write", false, "appliquer les corrections retenues (sinon simulation)")
	interactifF := fs.Bool("interactif", false, "confirmer chaque correction individuellement")
	sortieJSON := fs.Bool("json", false, "sortie JSON plutôt que texte (pour un usage scripté/agent)")
	fs.Parse(argvPourFlagSet(fs, argv))

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage : filiatium fix <fichier.ged> [--write] [--interactif] [--json]")
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

	// Référence AVANT toute correction : sert à détecter, après coup, si les
	// corrections en ont introduit de nouvelles (auto-vérification, voir plus bas).
	avant := executerRegles(g, rules.Registre, seuils)

	candidats := fix.Detecter(g)
	if len(candidats) == 0 {
		return finFix(*sortieJSON, chemin, nil, false, nil, "aucune correction mécanique applicable")
	}

	appliques := choisirEtAppliquer(candidats, *interactifF, *sortieJSON)
	if len(appliques) == 0 {
		return finFix(*sortieJSON, chemin, nil, false, nil, "aucune correction retenue")
	}

	if !*sortieJSON {
		mot := "simulée(s) — relancer avec --write pour écrire"
		if *ecrire {
			mot = "appliquée(s)"
		}
		fmt.Printf("\n%d correction(s) %s\n", len(appliques), mot)
	}

	if !*ecrire {
		return finFix(*sortieJSON, chemin, appliques, false, nil, "")
	}

	// Auto-vérification : on rejoue tout le registre sur le résultat et on refuse
	// d'écrire si une correction en a introduit de nouvelles — la simulation ne
	// prouve que l'intention, ceci prouve le résultat.
	apres := executerRegles(g, rules.Registre, seuils)
	nouveaux := signalementsNouveaux(avant, apres)
	if len(nouveaux) > 0 {
		if !*sortieJSON {
			fmt.Fprintln(os.Stderr, "\n⚠ écriture annulée : ces corrections introduisent de nouveaux signalements :")
			for _, n := range nouveaux {
				fmt.Fprintln(os.Stderr, "    "+n)
			}
		}
		return finFix(*sortieJSON, chemin, appliques, false, nouveaux, "écriture annulée : nouveaux signalements")
	}

	if _, err := g.Save(""); err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	return finFix(*sortieJSON, chemin, appliques, true, nil, "écrit.")
}

// finFix centralise la sortie finale de `fix`, texte ou JSON selon --json, et le
// code de retour (0 si rien à signaler, 1 si un refus/rollback a eu lieu). Pensé
// pour qu'un agent puisse s'appuyer sur --json plutôt que de parser du texte.
func finFix(sortieJSON bool, chemin string, appliques []fix.Correctif, ecrit bool, nouveaux []string, messageTexte string) int {
	if sortieJSON {
		type correctifJSON struct {
			Categorie string `json:"categorie"`
			Xref      string `json:"xref"`
			Diff      string `json:"diff"`
		}
		var cs []correctifJSON
		for _, c := range appliques {
			cs = append(cs, correctifJSON{Categorie: string(c.Categorie), Xref: c.Xref, Diff: c.Diff})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{
			"fichier": chemin, "correctifs": cs, "ecrit": ecrit,
			"nouveaux_signalements": nouveaux,
		})
	} else if messageTexte != "" {
		fmt.Println(messageTexte)
	}
	if len(nouveaux) > 0 {
		return 1
	}
	return 0
}

// choisirEtAppliquer applique chaque correctif après confirmation individuelle en
// mode --interactif (o/n/tout/aucun), ou toutes en une passe sinon. Chaque correctif
// choisi est appliqué immédiatement sur le Gedcom en mémoire — écrire ou non reste
// la décision de l'appelant.
func choisirEtAppliquer(candidats []fix.Correctif, interactifF, silencieux bool) []fix.Correctif {
	var appliques []fix.Correctif
	tout := false
	lecteur := bufio.NewScanner(os.Stdin)
	for _, c := range candidats {
		if interactifF && !tout {
			fmt.Printf("%s [o/n/tout/aucun] > ", c.Diff)
			if !lecteur.Scan() {
				break
			}
			switch strings.ToLower(strings.TrimSpace(lecteur.Text())) {
			case "tout", "t":
				tout = true
			case "o", "oui":
				// appliquer celui-ci seulement, suite normale ci-dessous
			case "aucun", "a":
				return appliques
			default:
				continue // "n"/"non"/vide/autre : sauter celui-ci
			}
		} else if !silencieux {
			fmt.Println(c.Diff)
		}
		c.Appliquer()
		appliques = append(appliques, c)
	}
	return appliques
}

// signalementsNouveaux renvoie, triés, les signalements présents dans `apres` mais
// absents de `avant` — peu importe qu'ils aient disparu ailleurs (une correction qui
// résout un signalement existant n'est jamais un problème).
func signalementsNouveaux(avant, apres map[string][]rules.Finding) []string {
	vusAvant := map[string]bool{}
	for _, findings := range avant {
		for _, f := range findings {
			vusAvant[f.Regle+"|"+f.Message] = true
		}
	}
	var out []string
	for _, findings := range apres {
		for _, f := range findings {
			if !vusAvant[f.Regle+"|"+f.Message] {
				out = append(out, f.Regle+" : "+f.Message)
			}
		}
	}
	sort.Strings(out)
	return out
}

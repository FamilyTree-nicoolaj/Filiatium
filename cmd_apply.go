package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/patch"
	"github.com/FamilyTree-nicoolaj/filiatium/rules"
)

const aideApply = `
filiatium apply <correctif.json> [options]

Applique un correctif déclaratif JSON : vérifie d'abord toutes les
préconditions (auto-invalidantes — un correctif déjà appliqué refuse de se
rejouer), puis exécute les opérations dans l'ordre. Le champ "cible" du
correctif est résolu relativement au dossier du fichier de correctif, pas au
répertoire courant.

Options :
  --write   écrire le résultat (sinon simulation)
  --json    sortie JSON plutôt que texte (pour un usage scripté/agent)

Opérations disponibles dans "operations" : set_event_date, add_citation,
add_fams, add_famc, add_lines (lignes neuves dans une fiche EXISTANTE — ex.
ajouter un BIRT/OCCU/NOTE à quelqu'un qui n'en avait pas ; ni "0 ..." — voir
add_record — ni remplacement d'une ligne déjà là — voir set_line), add_source,
add_individual, add_family, add_record (enregistrement entier, utilisé par
"merge --plan"), set_line, remove_line, touch_chan.

Exemple de correctif :
  {
    "cible": "family.ged",
    "justification": "Acte de mariage AD Tarn 5E123 vue 42",
    "preconditions": [{"xref": "F0111", "evenement": "MARR", "date_vaut": "5 JUN 1674"}],
    "operations": [
      {"op": "set_event_date", "xref": "F0111", "evenement": "MARR", "valeur": "27 MAY 1700"},
      {"op": "add_citation", "xref": "F0111", "source": "S0008", "evenement": "MARR"}
    ]
  }

Exemple add_lines (ajouter une naissance à quelqu'un qui n'en avait pas) :
  {"op": "add_lines", "xref": "I0042", "lignes": ["1 BIRT", "2 DATE 12 MAR 1805"]}

Exemples :
  filiatium apply correctif.json
  filiatium apply correctif.json --write
`

func init() {
	commandes = append(commandes, Commande{
		Nom:           "apply",
		Description:   "Appliquer un correctif déclaratif JSON (préconditions + opérations)",
		AideDetaillee: aideApply,
		Executer:      cmdApply,
	})
}

// flagsApply enregistre les options de `apply` sur fs — voir flagsCheck
// (cmd_check.go) pour pourquoi ceci est factorisé à part.
func flagsApply(fs *flag.FlagSet) (ecrire, sortieJSON *bool) {
	ecrire = fs.Bool("write", false, "écrire le résultat (sinon simulation)")
	sortieJSON = fs.Bool("json", false, "sortie JSON plutôt que texte (pour un usage scripté/agent)")
	return
}

func cmdApply(argv []string) int {
	if aideSiDemandee("apply", argv) {
		return 0
	}
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	ecrire, sortieJSON := flagsApply(fs)
	fs.Parse(argvPourFlagSet(fs, argv))

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage : filiatium apply <correctif.json> [--write] [--json]")
		return 2
	}
	cheminCorrectif := fs.Arg(0)

	c, err := patch.Charger(cheminCorrectif)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	if c.Cible == "" {
		fmt.Fprintf(os.Stderr, "erreur : %s : champ \"cible\" manquant\n", cheminCorrectif)
		return 2
	}
	cheminGed := resoudreCible(cheminCorrectif, c.Cible)

	g, err := gedcom.Load(cheminGed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	seuils, err := config.Charger(cheminGed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	avant := executerRegles(g, rules.Registre, seuils)

	if !*sortieJSON && c.Justification != "" {
		fmt.Println("justification :", c.Justification)
	}
	if err := c.Appliquer(g); err != nil {
		if *sortieJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.Encode(map[string]any{"erreur": err.Error()})
		} else {
			fmt.Fprintln(os.Stderr, "erreur :", err)
		}
		return 1
	}
	if !*sortieJSON {
		fmt.Printf("%d opération(s) appliquée(s) sur %s\n", len(c.Operations), cheminGed)
	}

	if !*ecrire {
		return finApply(*sortieJSON, cheminGed, len(c.Operations), false, nil)
	}

	// Même garde-fou que fix/add : refuser d'écrire si le correctif introduit un
	// signalement nouveau.
	apres := executerRegles(g, rules.Registre, seuils)
	nouveaux := signalementsNouveaux(avant, apres)
	if len(nouveaux) > 0 {
		if !*sortieJSON {
			fmt.Fprintln(os.Stderr, "\n⚠ écriture annulée : ce correctif introduit de nouveaux signalements :")
			for _, n := range nouveaux {
				fmt.Fprintln(os.Stderr, "    "+n)
			}
		}
		return finApply(*sortieJSON, cheminGed, len(c.Operations), false, nouveaux)
	}

	if _, err := g.Save(""); err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	return finApply(*sortieJSON, cheminGed, len(c.Operations), true, nil)
}

// finApply centralise la sortie finale de `apply`, texte ou JSON selon --json.
func finApply(sortieJSON bool, cheminGed string, nOperations int, ecrit bool, nouveaux []string) int {
	if sortieJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{
			"cible": cheminGed, "operations_appliquees": nOperations,
			"ecrit": ecrit, "nouveaux_signalements": nouveaux,
		})
	} else if !ecrit && len(nouveaux) == 0 {
		fmt.Println("(simulation — relancer avec --write pour écrire)")
	} else if ecrit {
		fmt.Println("écrit.")
	}
	if len(nouveaux) > 0 {
		return 1
	}
	return 0
}

// resoudreCible interprète "cible" relativement au dossier du correctif (pas au
// répertoire courant) : un correctif rangé à côté du GEDCOM continue de le trouver,
// quel que soit l'endroit d'où `filiatium apply` est invoqué.
func resoudreCible(cheminCorrectif, cible string) string {
	if filepath.IsAbs(cible) {
		return cible
	}
	return filepath.Join(filepath.Dir(cheminCorrectif), cible)
}

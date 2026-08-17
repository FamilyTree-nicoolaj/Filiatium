// filiatium — validation et correction de GEDCOM 5.5.1.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// version est renseignée au build via -ldflags "-X main.version=...".
var version = "dev"

// Commande est une sous-commande exécutable en CLI ou proposée dans le menu du mode
// guidé (interactif.go) — une seule table, lue par les deux interfaces, pour qu'elles
// ne puissent jamais diverger. Chaque cmd_*.go s'y ajoute via son propre init().
type Commande struct {
	Nom, Description string
	AideDetaillee    string // affiché par `filiatium <commande> help` / --help / -h
	Executer         func(argv []string) int
}

var commandes []Commande

func main() {
	os.Exit(executer(os.Args[1:]))
}

func executer(argv []string) int {
	if len(argv) == 0 {
		return lancerInteractif()
	}
	switch argv[0] {
	case "-h", "--help", "help":
		afficherAide()
		return 0
	case "--version":
		fmt.Println("filiatium", version)
		return 0
	case "--about":
		afficherAbout()
		return 0
	case "--ia":
		afficherManifesteIA()
		return 0
	}
	for _, c := range commandes {
		if c.Nom == argv[0] {
			return c.Executer(argv[1:])
		}
	}
	fmt.Fprintf(os.Stderr, "commande inconnue : %s\n\n", argv[0])
	afficherAide()
	return 2
}

// argvPourFlagSet réordonne argv pour que toutes les options passent avant les
// arguments positionnels, quel que soit leur agencement d'origine — "check
// fichier.ged --categorie liens" aussi bien que "merge --analyse base.ged
// apport.ged --plan p.json". Nécessaire parce que flag.Parse arrête son analyse dès
// le premier argument qui ne commence pas par "-", et traiterait sinon tout le reste
// (y compris de vraies options plus loin) comme des positionnels.
func argvPourFlagSet(fs *flag.FlagSet, argv []string) []string {
	var options, positionnels []string
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if !strings.HasPrefix(a, "-") {
			positionnels = append(positionnels, a)
			continue
		}
		options = append(options, a)
		if strings.Contains(a, "=") {
			continue // "--flag=valeur" : valeur déjà incluse dans ce jeton
		}
		nom := strings.TrimLeft(a, "-")
		fl := fs.Lookup(nom)
		if fl == nil {
			continue // option inconnue : laissée telle quelle, flag.Parse la signalera
		}
		if bv, ok := fl.Value.(interface{ IsBoolFlag() bool }); ok && bv.IsBoolFlag() {
			continue // booléen (ex. --write) : pas de valeur séparée à emporter
		}
		if i+1 < len(argv) {
			i++
			options = append(options, argv[i])
		}
	}
	return append(options, positionnels...)
}

func afficherAbout() {
	fmt.Println("filiatium", version)
	fmt.Println("Validation et correction de GEDCOM 5.5.1")
	fmt.Println()
	fmt.Println("Auteur  : Nicolas Jalibert")
	fmt.Println("Licence : MIT")
	fmt.Println("Source  : https://github.com/FamilyTree-nicoolaj/filiatium")
	fmt.Println()
	fmt.Println("filiatium <commande> help : aide complète de cette commande (options, exemples)")
}

func afficherAide() {
	fmt.Println("filiatium — validation et correction de GEDCOM 5.5.1")
	fmt.Println()
	fmt.Println("usage : filiatium <commande> [options]")
	fmt.Println()
	fmt.Println("commandes :")
	for _, c := range commandes {
		fmt.Printf("  %-10s %s\n", c.Nom, c.Description)
	}
	fmt.Println()
	fmt.Println("sans commande : mode guidé (menu interactif)")
	fmt.Println()
	fmt.Println("options : --version, --about, --help, --ia (manifeste JSON pour un agent)")
	fmt.Println("filiatium <commande> help : aide complète de cette commande (options, exemples)")
}

// aideSiDemandee affiche l'aide complète de la commande `nom` et renvoie true si
// l'utilisateur l'a demandée ("help", -h ou --help n'importe où dans argv) — à
// appeler en tout début de chaque cmdXxx, avant flag.Parse, pour qu'un tel argv ne
// soit jamais traité comme un chemin de fichier positionnel. Le binaire distribué
// seul (sans dépôt ni README à côté) doit pouvoir se documenter lui-même.
func aideSiDemandee(nom string, argv []string) bool {
	demandee := false
	for _, a := range argv {
		if a == "help" || a == "-h" || a == "--help" {
			demandee = true
			break
		}
	}
	if !demandee {
		return false
	}
	for _, c := range commandes {
		if c.Nom == nom {
			fmt.Println(strings.TrimSpace(c.AideDetaillee))
			return true
		}
	}
	return true
}

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/publish"
)

const aidePublish = `
filiatium publish <src.ged> <dst.ged> [options]

Retire d'un GEDCOM les faits datés (BIRT, MARR, DEAT...) qu'une règle de réalisme
juge invraisemblables et qu'AUCUNE citation SOUR ne vient étayer — écrit le
résultat dans un nouveau fichier dst.ged, sans jamais toucher src.ged. Un fait
sourcé (SOUR n'importe où sur l'individu/la famille concerné) n'est jamais
supprimé, même signalé.

Options :
  --niveau <strict|modere|large>  jusqu'où le doute profite au fait (défaut : strict)
  --interactif                    confirmer chaque suppression individuellement
  --write                         écrire dst.ged (sinon simulation : rapport seul)
  --json                          rapport en JSON plutôt qu'en texte

--niveau règle quelles règles de réalisme désignent des candidats, chaque niveau
incluant le précédent :
  strict   impossibilités strictes, sans seuil réglable : R10 (mariage postérieur
           au décès), R11 (date dans le futur), R12 (ordre baptême/naissance ou
           inhumation/décès incohérent)
  modere   + coïncidences suspectes, copier-coller probable : R1 (mariage
           identique à celui des parents), R5 (mêmes dates de naissance/décès)
  large    + les 8 règles restantes, basées sur un seuil réglable (filiatium.json) :
           R2, R3, R4, R6, R7, R8, R9, R13

Exemples :
  filiatium publish family.ged published.ged
  filiatium publish family.ged published.ged --niveau large --write
  filiatium publish family.ged published.ged --interactif --write
`

func init() {
	commandes = append(commandes, Commande{
		Nom:           "publish",
		Description:   "Retirer les faits invraisemblables et non sourcés d'un GEDCOM (--niveau strict|modere|large)",
		AideDetaillee: aidePublish,
		Executer:      cmdPublish,
	})
}

// flagsPublish enregistre les options de `publish` sur fs — voir flagsCheck
// (cmd_check.go) pour pourquoi ceci est factorisé à part.
func flagsPublish(fs *flag.FlagSet) (niveau *string, interactifF, ecrire, sortieJSON *bool) {
	niveau = fs.String("niveau", "strict", "jusqu'où le doute profite au fait : strict|modere|large")
	interactifF = fs.Bool("interactif", false, "confirmer chaque suppression individuellement")
	ecrire = fs.Bool("write", false, "écrire dst.ged (sinon simulation : rapport seul)")
	sortieJSON = fs.Bool("json", false, "rapport en JSON plutôt qu'en texte")
	return
}

func cmdPublish(argv []string) int {
	if aideSiDemandee("publish", argv) {
		return 0
	}
	fs := flag.NewFlagSet("publish", flag.ExitOnError)
	niveauFlag, interactifF, ecrire, sortieJSON := flagsPublish(fs)
	fs.Parse(argvPourFlagSet(fs, argv))

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage : filiatium publish <src.ged> <dst.ged> [--niveau strict|modere|large] [--interactif] [--write] [--json]")
		return 2
	}
	cheminSrc, cheminDst := fs.Arg(0), fs.Arg(1)
	if memeFichier(cheminSrc, cheminDst) {
		fmt.Fprintln(os.Stderr, "erreur : dst.ged doit être différent de src.ged — publish ne modifie jamais la source")
		return 2
	}

	niveau, err := publish.ParseNiveau(*niveauFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

	g, err := gedcom.Load(cheminSrc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	seuils, err := config.Charger(cheminSrc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

	candidats := publish.Calculer(g, niveau, seuils)
	aTraiter := candidats
	if *interactifF {
		aTraiter = choisirPublish(candidats)
	}

	n := 0
	if *ecrire {
		n = publish.Appliquer(g, aTraiter)
		if _, err := g.Save(cheminDst); err != nil {
			fmt.Fprintln(os.Stderr, "erreur :", err)
			return 2
		}
	}

	afficherPublish(cheminSrc, cheminDst, niveau, candidats, n, *ecrire, *sortieJSON)
	return 0
}

// choisirPublish confirme chaque candidat non sourcé individuellement (o/n/tout/aucun)
// — un candidat déjà sourcé n'est jamais proposé, il est protégé de toute façon.
// Même mécanique que choisirEtAppliquer (cmd_fix.go).
func choisirPublish(candidats []publish.Candidat) []publish.Candidat {
	var confirmes []publish.Candidat
	tout := false
	lecteur := bufio.NewScanner(os.Stdin)
	for _, c := range candidats {
		if c.Sourced {
			continue
		}
		if !tout {
			fmt.Printf("%s.%s [%s] %s [o/n/tout/aucun] > ", c.Xref, c.Tag, c.Regle, c.Message)
			if !lecteur.Scan() {
				break
			}
			switch strings.ToLower(strings.TrimSpace(lecteur.Text())) {
			case "tout", "t":
				tout = true
			case "o", "oui":
				// confirmé, suite normale ci-dessous
			case "aucun", "a":
				return confirmes
			default:
				continue // "n"/"non"/vide/autre : épargner celui-ci
			}
		}
		confirmes = append(confirmes, c)
	}
	return confirmes
}

func afficherPublish(cheminSrc, cheminDst string, niveau publish.Niveau, candidats []publish.Candidat, n int, ecrit, sortieJSON bool) {
	if sortieJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{
			"src": cheminSrc, "dst": cheminDst, "niveau": niveau.String(),
			"candidats": candidats, "supprimes": n, "ecrit": ecrit,
		})
		return
	}

	fmt.Printf("src    : %s\ndst    : %s\nniveau : %s\n\n", cheminSrc, cheminDst, niveau.String())

	nSources := 0
	fmt.Printf("=== candidats à la suppression (%d)\n", len(candidats))
	for _, c := range candidats {
		marque := ""
		if c.Sourced {
			marque = " [sourcé, épargné]"
			nSources++
		}
		fmt.Printf("    %s.%s [%s]%s %s\n", c.Xref, c.Tag, c.Regle, marque, c.Message)
	}
	fmt.Printf("\n%d candidat(s), dont %d sourcé(s) et jamais supprimé(s)\n", len(candidats), nSources)

	if ecrit {
		fmt.Printf("%d fait(s) supprimé(s) — écrit : %s\n", n, cheminDst)
	} else {
		fmt.Println("(simulation — relancer avec --write pour écrire dst.ged)")
	}
}

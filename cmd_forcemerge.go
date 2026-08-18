package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/merge"
)

const aideForcemerge = `
filiatium forcemerge <dst.ged> <srcA.ged> <srcB.ged> <xrefA1:xrefB1> [xrefA2:xrefB2 ...] [options]

Fusionne srcA et srcB directement dans dst.ged (jamais dans l'un des deux fichiers
source, qui restent tous les deux intacts sur disque), à partir d'ancres fournies
explicitement en "mode miroir" : chaque paire "xrefA:xrefB" déclare que l'individu
xrefA de srcA et l'individu xrefB de srcB sont la même personne.

Contrairement à "automerge" (contenu + score, ne fusionne jamais deux fiches qui se
ressemblent seulement, n'écrit jamais de GEDCOM lui-même), forcemerge écrit dst.ged
directement (avec --write) : les ancres ne sont jamais remises en cause, mais
l'appariement automatique (contenu, score, parenté) continue de tourner AUTOUR
d'elles, au niveau choisi par --fusionner — une ancre déclarée sur des parents aide
à retrouver leurs enfants, exactement comme le fait déjà automerge entre individus
détectés automatiquement.

Rien de ce qui existe dans srcA ou srcB ne disparaît silencieusement de dst.ged :
un fait qui diverge entre les deux (ex. deux dates de mariage différentes pour la
même famille) garde la valeur de srcA comme aujourd'hui, mais la valeur alternative
de srcB est en plus préservée en NOTE sur la fiche concernée — jamais perdue.

Options :
  --fusionner <niveau>  jusqu'où l'automatique complète les ancres, au-delà des
                         ancres elles-mêmes (toujours incluses) :
                         identiques|certaines|probables|tout (défaut : certaines)
  --write                écrire dst.ged (sinon simulation : rapport seul)
  --json                 rapport en JSON plutôt qu'en texte

Refuse d'écrire si la fusion introduit un signalement nouveau (voir "filiatium
check") — même garde que fix/add/apply/automerge.

Exemple :
  filiatium forcemerge dst.ged srcA.ged srcB.ged I1001:I4001 I0203:I4058 --write
`

func init() {
	commandes = append(commandes, Commande{
		Nom:           "forcemerge",
		Description:   "Fusionner directement deux GEDCOM dans un nouveau fichier à partir d'ancres déclarées (mode miroir)",
		AideDetaillee: aideForcemerge,
		Executer:      cmdForcemerge,
	})
}

// flagsForcemerge enregistre les options de `forcemerge` sur fs — voir flagsCheck
// (cmd_check.go) pour pourquoi ceci est factorisé à part.
func flagsForcemerge(fs *flag.FlagSet) (fusionner *string, ecrire, sortieJSON *bool) {
	fusionner = fs.String("fusionner", "certaines", "jusqu'où l'automatique complète les ancres : identiques|certaines|probables|tout")
	ecrire = fs.Bool("write", false, "écrire dst.ged (sinon simulation : rapport seul)")
	sortieJSON = fs.Bool("json", false, "rapport en JSON plutôt qu'en texte")
	return
}

func cmdForcemerge(argv []string) int {
	if aideSiDemandee("forcemerge", argv) {
		return 0
	}
	fs := flag.NewFlagSet("forcemerge", flag.ExitOnError)
	fusionnerFlag, ecrire, sortieJSON := flagsForcemerge(fs)
	fs.Parse(argvPourFlagSet(fs, argv))

	if fs.NArg() < 4 {
		fmt.Fprintln(os.Stderr, "usage : filiatium forcemerge <dst.ged> <srcA.ged> <srcB.ged> <xrefA:xrefB> [...] [options]")
		return 2
	}
	cheminDst, cheminA, cheminB := fs.Arg(0), fs.Arg(1), fs.Arg(2)
	if memeFichier(cheminDst, cheminA) || memeFichier(cheminDst, cheminB) {
		fmt.Fprintln(os.Stderr, "erreur : dst.ged doit être différent de srcA.ged et srcB.ged — forcemerge ne modifie jamais une source")
		return 2
	}

	forces, err := analyserPaires(fs.Args()[3:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

	niveau, err := merge.ParseNiveau(*fusionnerFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

	base, err := gedcom.Load(cheminA)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	apport, err := gedcom.Load(cheminB)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	if err := validerAncres(base, apport, forces); err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

	a := merge.Analyser(base, apport, niveau, forces)

	if *sortieJSON {
		afficherForcemergeJSON(cheminDst, cheminA, cheminB, forces, a, *ecrire && len(a.NouveauxApresMerge) == 0)
	} else {
		fmt.Printf("dst    : %s\nancres : %d fournie(s)\n\n", cheminDst, len(forces))
		afficherMergeTexte(cheminA, cheminB, a)
	}

	if len(a.NouveauxApresMerge) > 0 {
		if !*sortieJSON {
			fmt.Fprintln(os.Stderr, "\n⚠ écriture annulée : cette fusion introduit de nouveaux signalements (voir ci-dessus)")
		}
		return 1
	}

	if !*ecrire {
		if !*sortieJSON {
			fmt.Println("\n(simulation — relancer avec --write pour écrire dst.ged)")
		}
		return 0
	}

	plan := merge.PlanForce(base, apport, forces, "", niveau)
	if err := plan.Appliquer(base); err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	if _, err := base.Save(cheminDst); err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	if !*sortieJSON {
		fmt.Println("\nécrit :", cheminDst)
	}
	return 0
}

// memeFichier compare deux chemins par leur forme absolue — pour que "dst.ged" et
// "./dst.ged", par exemple, soient bien reconnus comme le même fichier.
func memeFichier(a, b string) bool {
	aa, erra := filepath.Abs(a)
	bb, errb := filepath.Abs(b)
	if erra != nil || errb != nil {
		return a == b
	}
	return aa == bb
}

// analyserPaires décode les positionnels "xrefA:xrefB" en une table xref d'apport ->
// xref de base (convention du paquet merge) ; refuse une paire malformée ou un xref
// réutilisé dans plus d'une paire d'un même côté (ambiguïté qu'aucune heuristique ne
// doit trancher à la place de l'utilisateur).
func analyserPaires(pairesArgs []string) (map[string]string, error) {
	forces := map[string]string{}
	vusA, vusB := map[string]bool{}, map[string]bool{}
	for _, p := range pairesArgs {
		parts := strings.SplitN(p, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("paire invalide %q (attendu xrefA:xrefB)", p)
		}
		xrefA, xrefB := strings.Trim(parts[0], "@"), strings.Trim(parts[1], "@")
		if vusA[xrefA] {
			return nil, fmt.Errorf("%s (srcA) apparaît dans plus d'une paire", xrefA)
		}
		if vusB[xrefB] {
			return nil, fmt.Errorf("%s (srcB) apparaît dans plus d'une paire", xrefB)
		}
		vusA[xrefA], vusB[xrefB] = true, true
		forces[xrefB] = xrefA
	}
	return forces, nil
}

// validerAncres vérifie que chaque xref d'une ancre désigne bien un individu existant
// du bon fichier — preparer()/apparier() font confiance à forces sans revérifier
// (voir merge/merge.go), c'est ici, une fois pour toutes, que la garantie est établie.
func validerAncres(base, apport *gedcom.Gedcom, forces map[string]string) error {
	for xrefApport, xrefBase := range forces {
		rb, ok := base.Get(xrefBase)
		if !ok || rb.Tag != "INDI" {
			return fmt.Errorf("%s n'est pas un individu de srcA", xrefBase)
		}
		ra, ok := apport.Get(xrefApport)
		if !ok || ra.Tag != "INDI" {
			return fmt.Errorf("%s n'est pas un individu de srcB", xrefApport)
		}
	}
	return nil
}

func afficherForcemergeJSON(cheminDst, cheminA, cheminB string, forces map[string]string, a *merge.Analyse, ecrit bool) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{
		"dst": cheminDst, "srcA": cheminA, "srcB": cheminB,
		"ancres_fournies": len(forces), "analyse": a, "ecrit": ecrit,
	})
}

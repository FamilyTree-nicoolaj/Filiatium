package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/merge"
)

const aideMerge = `
filiatium merge --analyse <base.ged> <apport.ged> [options]

Analyse si deux GEDCOM sont fusionnables — n'écrit jamais de GEDCOM lui-même.
Produit un rapport : collisions de xref, appariements d'individus classés
certaine/probable/à examiner (avec les critères qui ont joué et les conflits de
faits éventuels), et contradictions qu'introduirait une fusion mécanique
(rejoue le registre de règles sur une concaténation renumérotée). Avec --plan,
écrit en plus un plan de fusion déclaratif exécutable via "filiatium apply".

--analyse est obligatoire : c'est le seul mode disponible pour l'instant.

Options :
  --analyse         obligatoire — analyser la fusion
  --plan <fichier>  écrire le plan de fusion déclaratif (JSON, pour apply) dans ce fichier
  --prefixe <p>     préfixe de renumérotation proposé pour l'apport (défaut : B)
  --json            rapport en JSON plutôt qu'en texte

Le plan ne fusionne aucune fiche identifiée comme doublon automatiquement :
c'est un jugement humain, à faire à la lecture des appariements "certaine".

Exemples :
  filiatium merge --analyse family.ged secondary_trees/sicard-binas-1779.ged
  filiatium merge --analyse base.ged apport.ged --plan fusion.json --prefixe B
  filiatium apply fusion.json --write   # après relecture du rapport
`

func init() {
	commandes = append(commandes, Commande{
		Nom:           "merge",
		Description:   "Analyser si deux GEDCOM sont fusionnables (--analyse <base.ged> <apport.ged>)",
		AideDetaillee: aideMerge,
		Executer:      cmdMerge,
	})
}

func cmdMerge(argv []string) int {
	if aideSiDemandee("merge", argv) {
		return 0
	}
	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	analyse := fs.Bool("analyse", false, "analyser la fusion (seul mode disponible : merge n'écrit jamais de GEDCOM)")
	sortiePlan := fs.String("plan", "", "écrire le plan de fusion déclaratif (JSON, pour `apply`) dans ce fichier")
	prefixe := fs.String("prefixe", merge.PrefixeParDefaut, "préfixe de renumérotation proposé pour l'apport")
	sortieJSON := fs.Bool("json", false, "rapport en JSON plutôt qu'en texte")
	fs.Parse(argvPourFlagSet(fs, argv))

	if !*analyse {
		fmt.Fprintln(os.Stderr, "usage : filiatium merge --analyse <base.ged> <apport.ged> [--plan fusion.json] [--prefixe B]")
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage : filiatium merge --analyse <base.ged> <apport.ged>")
		return 2
	}
	cheminBase, cheminApport := fs.Arg(0), fs.Arg(1)

	base, err := gedcom.Load(cheminBase)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	apport, err := gedcom.Load(cheminApport)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

	a := merge.Analyser(base, apport)

	if *sortiePlan != "" {
		plan := merge.Plan(base, apport, cheminBase, *prefixe)
		octets, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "erreur :", err)
			return 2
		}
		if err := os.WriteFile(*sortiePlan, octets, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "erreur :", err)
			return 2
		}
		fmt.Printf("plan de fusion écrit : %s (%d opération(s), à relire avant `apply --write`)\n",
			*sortiePlan, len(plan.Operations))
	}

	if *sortieJSON {
		afficherMergeJSON(cheminBase, cheminApport, a)
	} else {
		afficherMergeTexte(cheminBase, cheminApport, a)
	}

	if len(a.NouveauxApresMerge) > 0 {
		return 1
	}
	return 0
}

func afficherMergeTexte(cheminBase, cheminApport string, a *merge.Analyse) {
	fmt.Printf("base   : %s\napport : %s\n\n", cheminBase, cheminApport)

	fmt.Println("=== collisions de xref")
	fmt.Printf("    INDI %d, FAM %d, SOUR %d — préfixe de renumérotation suggéré : %q\n\n",
		a.Collisions.Individus, a.Collisions.Familles, a.Collisions.Sources, a.PrefixeSuggere)

	fmt.Printf("=== appariements (%d)\n", len(a.Appariements))
	for _, app := range a.Appariements {
		fmt.Printf("    [%s] score %d — %s (%s) <-> %s (%s)\n",
			app.Classe, app.Score, app.XrefBase, app.NomBase, app.XrefApport, app.NomApport)
		for _, c := range app.Criteres {
			fmt.Println("        + " + c)
		}
		for _, c := range app.Conflits {
			fmt.Println("        ⚠ " + c)
		}
	}
	fmt.Println()

	fmt.Printf("=== contradictions introduites par la fusion mécanique (%d)\n", len(a.NouveauxApresMerge))
	for _, n := range a.NouveauxApresMerge {
		fmt.Println("    " + n)
	}
	fmt.Println()

	fmt.Println("verdict :", a.Verdict)
}

func afficherMergeJSON(cheminBase, cheminApport string, a *merge.Analyse) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{"base": cheminBase, "apport": cheminApport, "analyse": a})
}

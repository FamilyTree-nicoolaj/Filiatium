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
Identifie les enregistrements par leur CONTENU, jamais par leurs xref (qui peuvent
coïncider par accident, comme deux exports d'une même base Gramps, ou diverger
totalement). Produit un rapport (collisions de xref, appariements d'individus
classés certaine/probable/à examiner avec leurs critères et conflits, contradictions
qu'introduirait la fusion) et, avec --plan, un plan de fusion déclaratif exécutable
via "filiatium apply" : il réutilise tel quel ce qui est déjà identique dans la base,
complète les fiches appariées avec les lignes qui leur manquent (ex. une famille dont
un export a gardé les enfants et l'autre les parents), et n'insère vraiment de
nouveaux enregistrements que pour ce qui reste — en renumérotant seulement en cas de
collision réelle de xref.

--analyse est obligatoire : c'est le seul mode disponible pour l'instant.

Options :
  --analyse            obligatoire — analyser la fusion
  --plan <fichier>      écrire le plan de fusion déclaratif (JSON, pour apply) dans ce fichier
  --fusionner <niveau>  ce que le plan incorpore : identiques|certaines|probables|tout (défaut : certaines)
  --json                rapport en JSON plutôt qu'en texte

--fusionner définit jusqu'où le plan fusionne automatiquement (chaque niveau inclut
le précédent) :
  identiques  uniquement le contenu octet-identique entre les deux fichiers — aucun jugement
  certaines   + les appariements certains (individus et familles qui en découlent)
  probables   + les appariements probables (score 40-69)
  tout        + les appariements "à examiner"

Un appariement au-delà du niveau choisi reste visible au rapport mais n'entre jamais
dans le plan : un bloc en conflit de valeur (ex. deux dates de mariage différentes)
n'est jamais appliqué non plus, quel que soit le niveau — c'est un jugement humain.

Exemples :
  filiatium merge --analyse family.ged secondary_trees/sicard-binas-1779.ged
  filiatium merge --analyse base.ged apport.ged --plan fusion.json --fusionner certaines
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

// flagsMerge enregistre les options de `merge` sur fs — voir flagsCheck
// (cmd_check.go) pour pourquoi ceci est factorisé à part.
func flagsMerge(fs *flag.FlagSet) (analyse *bool, sortiePlan, fusionner *string, sortieJSON *bool) {
	analyse = fs.Bool("analyse", false, "analyser la fusion (seul mode disponible : merge n'écrit jamais de GEDCOM)")
	sortiePlan = fs.String("plan", "", "écrire le plan de fusion déclaratif (JSON, pour `apply`) dans ce fichier")
	fusionner = fs.String("fusionner", "certaines", "ce que le plan incorpore : identiques|certaines|probables|tout")
	sortieJSON = fs.Bool("json", false, "rapport en JSON plutôt qu'en texte")
	return
}

func cmdMerge(argv []string) int {
	if aideSiDemandee("merge", argv) {
		return 0
	}
	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	analyse, sortiePlan, fusionnerFlag, sortieJSON := flagsMerge(fs)
	fs.Parse(argvPourFlagSet(fs, argv))

	if !*analyse {
		fmt.Fprintln(os.Stderr, "usage : filiatium merge --analyse <base.ged> <apport.ged> [--plan fusion.json] [--fusionner certaines]")
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage : filiatium merge --analyse <base.ged> <apport.ged>")
		return 2
	}
	cheminBase, cheminApport := fs.Arg(0), fs.Arg(1)

	niveau, err := merge.ParseNiveau(*fusionnerFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

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

	a := merge.Analyser(base, apport, niveau)

	if *sortiePlan != "" {
		plan := merge.Plan(base, apport, cheminBase, niveau)
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
	fmt.Printf("    INDI %d, FAM %d, SOUR %d\n\n",
		a.Collisions.Individus, a.Collisions.Familles, a.Collisions.Sources)

	fmt.Printf("=== fusion proposée (niveau %q)\n", a.Niveau)
	fmt.Printf("    %d réutilisé(s) à l'identique, %d fiche(s) complétée(s), %d nouveau(x), %d renuméroté(s)\n",
		a.Identiques, a.Completees, a.Nouveaux, a.Renumerotes)
	if len(a.ConflitsNonAppliques) > 0 {
		fmt.Printf("    %d bloc(s) en conflit, non appliqué(s) — à arbitrer :\n", len(a.ConflitsNonAppliques))
		for _, c := range a.ConflitsNonAppliques {
			fmt.Println("        ⚠ " + c)
		}
	}
	fmt.Println()

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

	fmt.Printf("=== contradictions introduites par la fusion (%d)\n", len(a.NouveauxApresMerge))
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

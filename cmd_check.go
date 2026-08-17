package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/rules"
)

const aideCheckEntete = `
filiatium check <fichier.ged> [options]

Vérifie un GEDCOM sous le registre de règles complet (structure, liens,
doublons, réalisme). Code de sortie : 0 si rien à signaler, 1 sinon.

Options :
  --avant <ref>      comparer les comptes par type et les xref ajoutés/supprimés
                      avec cette version de référence du même fichier
  --regle L1,L2,...   n'exécuter que ces règles (identifiants ci-dessous)
  --categorie <nom>   n'exécuter que cette catégorie : structure, liens, doublons, realisme
  --json              sortie JSON plutôt que texte

Règles :`

const aideCheckPied = `
Exemples :
  filiatium check family.ged
  filiatium check family.ged --categorie realisme
  filiatium check family.ged --regle L1,L2,D1
  filiatium check family.ged --avant family.ged.bak-2026-08-04
  filiatium check family.ged --json
`

// afficherAideCheck imprime l'aide complète de `check`, avec le tableau des règles
// généré en itérant rules.Registre plutôt que recopié en dur — il ne peut donc
// jamais diverger si une règle est ajoutée, renommée ou déplacée de catégorie.
func afficherAideCheck() {
	fmt.Println(strings.TrimSpace(aideCheckEntete))
	categorieCourante := ""
	for _, r := range rules.Registre {
		if r.Categorie != categorieCourante {
			categorieCourante = r.Categorie
			fmt.Printf("  %s\n", strings.ToUpper(categorieCourante))
		}
		fmt.Printf("    %-4s %s\n", r.ID, r.Titre)
	}
	fmt.Println(strings.TrimSpace(aideCheckPied))
}

func init() {
	commandes = append(commandes, Commande{
		Nom:         "check",
		Description: "Vérifier un GEDCOM (structure, liens, doublons, réalisme)",
		Executer:    cmdCheck,
	})
}

func cmdCheck(argv []string) int {
	for _, a := range argv {
		if a == "help" || a == "-h" || a == "--help" {
			afficherAideCheck()
			return 0
		}
	}
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	avant := fs.String("avant", "", "comparer les comptes par type avec cette version de référence")
	regleFlag := fs.String("regle", "", "règles à exécuter, séparées par des virgules (ex. L1,L2)")
	categorie := fs.String("categorie", "", "catégorie à exécuter (structure, liens, doublons, realisme)")
	sortieJSON := fs.Bool("json", false, "sortie JSON plutôt que texte")
	fs.Parse(argvPourFlagSet(fs, argv))

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage : filiatium check <fichier.ged> [--avant <ref>] [--regle L1,L2] [--categorie liens] [--json]")
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
	regles := reglesSelectionnees(*regleFlag, *categorie)
	resultats := executerRegles(g, regles, seuils)

	ok := true
	for _, findings := range resultats {
		if len(findings) > 0 {
			ok = false
		}
	}

	if *sortieJSON {
		afficherJSON(chemin, resultats)
	} else {
		afficherTexte(chemin, g, resultats)
	}

	if *avant != "" && !comparerAvant(chemin, *avant) {
		ok = false
	}

	if ok {
		return 0
	}
	return 1
}

func reglesSelectionnees(regleFlag, categorie string) []rules.Regle {
	if regleFlag != "" {
		voulues := map[string]bool{}
		for _, id := range strings.Split(regleFlag, ",") {
			voulues[strings.TrimSpace(id)] = true
		}
		var out []rules.Regle
		for _, r := range rules.Registre {
			if voulues[r.ID] {
				out = append(out, r)
			}
		}
		return out
	}
	if categorie != "" {
		var out []rules.Regle
		for _, r := range rules.Registre {
			if r.Categorie == categorie {
				out = append(out, r)
			}
		}
		return out
	}
	return rules.Registre
}

// executerRegles exécute chaque règle et déduplique+trie ses signalements par
// message — comme sorted(set(fn(g))) dans les scripts Python d'origine.
func executerRegles(g *gedcom.Gedcom, regles []rules.Regle, seuils config.Seuils) map[string][]rules.Finding {
	out := make(map[string][]rules.Finding, len(regles))
	for _, r := range regles {
		vus := map[string]bool{}
		var dedupe []rules.Finding
		for _, f := range r.Verifie(g, seuils) {
			if !vus[f.Message] {
				vus[f.Message] = true
				dedupe = append(dedupe, f)
			}
		}
		sort.Slice(dedupe, func(i, j int) bool { return dedupe[i].Message < dedupe[j].Message })
		out[r.ID] = dedupe
	}
	return out
}

func afficherTexte(chemin string, g *gedcom.Gedcom, resultats map[string][]rules.Finding) {
	fmt.Printf("%s — %d individus, %d familles\n\n", chemin, len(g.Individus()), len(g.Familles()))
	categorieCourante := ""
	total := 0
	for _, r := range rules.Registre {
		findings, testee := resultats[r.ID]
		if !testee {
			continue
		}
		if r.Categorie != categorieCourante {
			categorieCourante = r.Categorie
			fmt.Printf("=== %s\n", strings.ToUpper(categorieCourante))
		}
		fmt.Printf("### %s. %s — %d signalement(s)\n", r.ID, r.Titre, len(findings))
		for _, f := range findings {
			fmt.Println("    " + f.Message)
		}
		total += len(findings)
	}
	fmt.Printf("\ntotal : %d signalement(s)\n", total)
}

func afficherJSON(chemin string, resultats map[string][]rules.Finding) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{"fichier": chemin, "resultats": resultats})
}

// comparerAvant reproduit le `--avant` de valider.py : compte des enregistrements par
// type et xref ajoutés/supprimés entre deux versions du même fichier. Un compte
// d'INDI/FAM qui bouge, ou un xref supprimé, fait échouer la comparaison — c'est ce
// qui signale un dégât après une simple correction de date.
func comparerAvant(cheminApres, cheminAvant string) bool {
	gApres, err := gedcom.Load(cheminApres)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return false
	}
	gAvant, err := gedcom.Load(cheminAvant)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return false
	}

	typesApres, typesAvant := compterTypes(gApres), compterTypes(gAvant)
	fmt.Println("\n--- comparaison")
	ok := true
	for _, t := range clesTriees(typesAvant, typesApres) {
		a, b := typesAvant[t], typesApres[t]
		marque := "  "
		if a != b {
			marque = "⚠ "
		}
		fmt.Printf("    %s%-5s %5d -> %5d", marque, t, a, b)
		if a != b {
			fmt.Printf("  (%+d)", b-a)
		}
		fmt.Println()
		if a != b && (t == "INDI" || t == "FAM") {
			ok = false
		}
	}

	defAvant, defApres := gAvant.ParXref(), gApres.ParXref()
	var ajoutes, retires []string
	for x := range defApres {
		if _, present := defAvant[x]; !present {
			ajoutes = append(ajoutes, x)
		}
	}
	for x := range defAvant {
		if _, present := defApres[x]; !present {
			retires = append(retires, x)
		}
	}
	sort.Strings(ajoutes)
	sort.Strings(retires)
	fmt.Printf("    identifiants ajoutés   : %s\n", formatListe(ajoutes))
	fmt.Printf("    identifiants supprimés : %s\n", formatListe(retires))
	if len(retires) > 0 {
		ok = false
	}
	return ok
}

func compterTypes(g *gedcom.Gedcom) map[string]int {
	m := map[string]int{}
	for _, r := range g.Records {
		m[r.Tag]++
	}
	return m
}

func clesTriees(a, b map[string]int) []string {
	vus := map[string]bool{}
	var out []string
	for _, m := range []map[string]int{a, b} {
		for k := range m {
			if !vus[k] {
				vus[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

func formatListe(xs []string) string {
	if len(xs) == 0 {
		return "aucun"
	}
	return "[" + strings.Join(xs, ", ") + "]"
}

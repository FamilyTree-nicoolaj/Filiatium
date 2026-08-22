package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/geneanet"
	"github.com/FamilyTree-nicoolaj/filiatium/rules"
)

const aideImport = `
filiatium import <dst.ged> <fiche1.html> [fiche2.html ...] [options]

Construit un arbre GEDCOM complet à partir de pages HTML de fiches individuelles
Geneanet (parents, union(s)/enfants, frères et sœurs, notes, sources), en dédupliquant
automatiquement les personnes qui apparaissent sur plusieurs fiches (même patronyme et
prénom, et — quand elle est connue des deux côtés — même année de naissance).

Chaque fichier d'entrée est le code source HTML de la page individuelle telle que vue
dans le navigateur (menu "Enregistrer sous..." ou "Afficher la source de la page") —
jamais une capture d'écran, jamais un accès réseau direct à Geneanet.

Écrit toujours vers un nouveau fichier dst.ged, jamais dans un fichier déjà existant.
Rejoue le registre de règles ("filiatium check") avant/après la construction ; refuse
d'écrire si elle introduit un signalement structure/liens/doublons (un vrai défaut).
Un signalement de réalisme (ex. R13, aucun décès enregistré) reste affiché dans le
rapport sans bloquer l'écriture : sur un arbre neuf, comparé à un fichier vide, c'est
un retour légitime sur des données réellement incomplètes, pas un défaut introduit
par la construction.

Options :
  --auteur    utilisatrice/contributrice Geneanet source de la fiche (ex.
              "Sylvie DUJARDIN (sylvied58)"), attribuée à la source Geneanet créée
              pour cet import
  --force     "D1" : fusionne automatiquement chaque paire de FAM signalée par D1
              (conjoint commun et enfant(s) commun(s), donc probablement la même union
              décrite sur deux fiches) avant d'écrire — la famille conservée récupère
              le HUSB/WIFE/CHIL et tout autre fait (MARR, NOTE, SOUR...) que l'autre
              connaissait en plus ; rien n'est perdu
  --write     écrire dst.ged (sinon simulation : rapport seul)
  --json      rapport en JSON plutôt qu'en texte

Exemple :
  filiatium import arbre.ged fiche*.html --auteur "Sylvie DUJARDIN (sylvied58)" --write
`

func init() {
	commandes = append(commandes, Commande{
		Nom:           "import",
		Description:   "Construire un GEDCOM depuis des pages HTML Geneanet",
		AideDetaillee: aideImport,
		Executer:      cmdImport,
	})
}

// flagsImport enregistre les options de `import` sur fs — voir flagsCheck
// (cmd_check.go) pour pourquoi ceci est factorisé à part.
func flagsImport(fs *flag.FlagSet) (auteur, force *string, ecrire, sortieJSON *bool) {
	auteur = fs.String("auteur", "", `utilisatrice/contributrice Geneanet source de la fiche, ex. "Sylvie DUJARDIN (sylvied58)"`)
	force = fs.String("force", "", `"D1" : fusionne automatiquement les paires de FAM signalées par D1`)
	ecrire = fs.Bool("write", false, "écrire dst.ged (sinon simulation)")
	sortieJSON = fs.Bool("json", false, "rapport en JSON plutôt qu'en texte")
	return
}

// sourceImport reconnaît et parse un format de fiche donné (un site de généalogie) —
// une seule entrée aujourd'hui (Geneanet), mais une deuxième s'ajoute ici sans toucher
// au reste de cmdImport, même convention que commandes ([]Commande, main.go) et
// rules.Registre ([]Regle, rules/rules.go) : le registre vit chez le consommateur, pas
// chez la source (évite un cycle d'import si une source devait un jour référencer un
// type partagé).
type sourceImport struct {
	Nom      string
	Detecter func(contenu []byte) bool
	Parser   func(contenu []byte) (*geneanet.Fiche, error)
}

var sourcesImport = []sourceImport{
	{Nom: "geneanet", Detecter: geneanet.EstFicheGeneanet, Parser: geneanet.ParserHTML},
}

// detecterSource choisit la source d'une fiche par son CONTENU, jamais par l'extension
// du fichier — le navigateur de l'utilisateur peut enregistrer en .html, .htm, voire
// .txt selon sa configuration.
func detecterSource(contenu []byte) (*sourceImport, error) {
	var trouvee *sourceImport
	for i := range sourcesImport {
		if sourcesImport[i].Detecter(contenu) {
			if trouvee != nil {
				return nil, fmt.Errorf("plusieurs sources reconnaissent ce contenu (%s, %s) — ambigu",
					trouvee.Nom, sourcesImport[i].Nom)
			}
			trouvee = &sourcesImport[i]
		}
	}
	if trouvee == nil {
		return nil, fmt.Errorf("format non reconnu — attendu : page HTML d'une fiche individuelle Geneanet " +
			"(« Enregistrer sous » ou « Afficher la source » depuis le navigateur)")
	}
	return trouvee, nil
}

// reglesForcables sont les règles que --force sait effectivement appliquer —
// uniquement D1 (fusion de FAM quasi-doublons) pour l'instant : aucune autre règle du
// registre n'a une correction "fusionner deux enregistrements" qui ait un sens.
var reglesForcables = map[string]bool{"D1": true}

// parseForce décode l'option --force ("" ou une liste séparée par des virgules, ex.
// "D1") et refuse toute règle que reglesForcables ne sait pas appliquer.
func parseForce(s string) (map[string]bool, error) {
	out := map[string]bool{}
	if s == "" {
		return out, nil
	}
	for _, regle := range strings.Split(s, ",") {
		regle = strings.TrimSpace(regle)
		if !reglesForcables[regle] {
			return nil, fmt.Errorf("--force ne sait appliquer que %s (reçu %q)", "D1", regle)
		}
		out[regle] = true
	}
	return out, nil
}

// fusionnerD1 fusionne chaque paire de FAM signalée par D1 (voir rules.D1 et
// gedcom.FusionnerFamilles) : Xrefs[0]/[1] sont toujours les deux FAM de la paire (voir
// rules/doublons.go). Une paire dont l'un des deux xref a déjà disparu (fusionné comme
// second membre d'une paire précédente dans ce même passage) est ignorée plutôt que de
// tenter une fusion sur un xref déjà supprimé. Une paire que FusionnerFamilles refuse
// (HUSB/WIFE connu des deux côtés mais différent — pas un doublon mécanique) est aussi
// ignorée plutôt que d'interrompre les autres : elle réapparaîtra en D1 dans le rapport
// "après", donc --write restera bloqué dessus comme sans --force, ignores explique
// pourquoi au lieu de laisser deviner.
func fusionnerD1(g *gedcom.Gedcom, seuils config.Seuils) (fusionnees int, ignorees []string) {
	for _, f := range rules.D1(g, seuils) {
		x1, x2 := f.Xrefs[0], f.Xrefs[1]
		if !g.Contains(x1) || !g.Contains(x2) {
			continue
		}
		if err := g.FusionnerFamilles(x1, x2); err != nil {
			ignorees = append(ignorees, err.Error())
			continue
		}
		fusionnees++
	}
	return fusionnees, ignorees
}

func cmdImport(argv []string) int {
	if aideSiDemandee("import", argv) {
		return 0
	}
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	auteur, force, ecrire, sortieJSON := flagsImport(fs)
	fs.Parse(argvPourFlagSet(fs, argv))

	forcees, err := parseForce(*force)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage : filiatium import <dst.ged> <fiche1.html> [fiche2.html ...] [options]")
		return 2
	}
	cheminDst, sources := fs.Arg(0), fs.Args()[1:]
	for _, src := range sources {
		if memeFichier(cheminDst, src) {
			fmt.Fprintln(os.Stderr, "erreur : dst.ged doit être différent des fiches d'entrée")
			return 2
		}
	}

	var fiches []*geneanet.Fiche
	for _, src := range sources {
		contenu, err := os.ReadFile(src)
		if err != nil {
			fmt.Fprintln(os.Stderr, "erreur :", err)
			return 2
		}
		source, err := detecterSource(contenu)
		if err != nil {
			fmt.Fprintf(os.Stderr, "erreur : %s : %v\n", src, err)
			return 2
		}
		f, err := source.Parser(contenu)
		if err != nil {
			fmt.Fprintf(os.Stderr, "erreur : %s : %v\n", src, err)
			return 2
		}
		fiches = append(fiches, f)
	}

	g := gedcom.Nouveau()
	seuils, err := config.Charger(cheminDst)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	avant := executerRegles(g, rules.Registre, seuils)

	rapport, err := geneanet.Construire(g, fiches, *auteur)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}

	nFusionsD1 := 0
	var ignoreesD1 []string
	if forcees["D1"] {
		nFusionsD1, ignoreesD1 = fusionnerD1(g, seuils)
	}

	apres := executerRegles(g, rules.Registre, seuils)
	// La garde d'écriture compare ici à un arbre vide (gedcom.Nouveau) plutôt qu'à un
	// arbre existant : un signalement de réalisme (ex. R13, "aucun décès enregistré")
	// est alors quasi certain sur n'importe quelle capture réelle (beaucoup
	// d'ascendants n'ont simplement pas de date de décès connue) — un retour légitime
	// sur les données, pas un défaut introduit par la construction. Seules les
	// catégories structure/liens/doublons (des défauts réels : lien cassé, doublon
	// structurel) bloquent --write ; le réalisme reste affiché dans le rapport.
	nouveaux := signalementsNouveaux(sansRealisme(avant), sansRealisme(apres))

	if *sortieJSON {
		afficherImportJSON(cheminDst, sources, rapport, apres, nFusionsD1, ignoreesD1, *ecrire && len(nouveaux) == 0)
	} else {
		afficherImportTexte(cheminDst, sources, rapport, apres, nFusionsD1, ignoreesD1)
	}

	if len(nouveaux) > 0 {
		if !*sortieJSON {
			fmt.Fprintln(os.Stderr, "\n⚠ écriture annulée : cette construction introduit de nouveaux signalements (voir \"filiatium check\")")
			for _, n := range nouveaux {
				fmt.Fprintln(os.Stderr, "    "+n)
			}
		}
		return 1
	}

	if !*ecrire {
		if !*sortieJSON {
			fmt.Println("\n(simulation — relancer avec --write pour écrire dst.ged)")
		}
		return 0
	}

	if _, err := g.Save(cheminDst); err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		return 2
	}
	if !*sortieJSON {
		fmt.Println("\nécrit :", cheminDst)
	}
	return 0
}

// sansRealisme retire les signalements de catégorie "realisme" d'un résultat de
// executerRegles — voir le commentaire sur nouveaux dans cmdImport.
func sansRealisme(m map[string][]rules.Finding) map[string][]rules.Finding {
	out := make(map[string][]rules.Finding, len(m))
	for _, r := range rules.Registre {
		if r.Categorie == "realisme" {
			continue
		}
		if findings, ok := m[r.ID]; ok {
			out[r.ID] = findings
		}
	}
	return out
}

func afficherImportTexte(cheminDst string, sources []string, rapport *geneanet.Rapport, apres map[string][]rules.Finding, nFusionsD1 int, ignoreesD1 []string) {
	fmt.Printf("dst     : %s\nfiches  : %d\n\n", cheminDst, len(sources))
	fmt.Printf("individus créés : %d\nfamilles créées  : %d\nsources créées   : %d\n", rapport.Individus, rapport.Familles, rapport.Sources)
	for _, a := range rapport.Ambigus {
		fmt.Println("  ⚠", a)
	}
	if nFusionsD1 > 0 {
		fmt.Printf("fusions D1 forcées : %d\n", nFusionsD1)
	}
	for _, ign := range ignoreesD1 {
		fmt.Println("  ⚠ D1 non fusionnée :", ign)
	}
	total := 0
	for _, findings := range apres {
		total += len(findings)
	}
	fmt.Printf("\nfiltre \"check\" après construction : %d signalement(s)\n", total)
}

func afficherImportJSON(cheminDst string, sources []string, rapport *geneanet.Rapport, apres map[string][]rules.Finding, nFusionsD1 int, ignoreesD1 []string, ecrit bool) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{
		"dst": cheminDst, "fiches": sources, "rapport": rapport, "check_apres": apres,
		"fusions_d1_forcees": nFusionsD1, "d1_non_fusionnees": ignoreesD1, "ecrit": ecrit,
	})
}

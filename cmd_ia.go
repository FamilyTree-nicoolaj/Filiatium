package main

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"sort"

	"github.com/FamilyTree-nicoolaj/filiatium/rules"
)

// optionIA décrit une option de ligne de commande pour un agent IA. Construite par
// introspection du *flag.FlagSet réel de la commande (fs.VisitAll dans
// commandeIAPour), jamais recopiée à la main : elle ne peut donc pas diverger des
// options qui existent vraiment.
type optionIA struct {
	Nom         string `json:"nom"`
	Type        string `json:"type"` // "string" ou "bool"
	Defaut      string `json:"defaut"`
	Description string `json:"description"`
}

type positionnelIA struct {
	Nom         string `json:"nom"`
	Description string `json:"description"`
	Obligatoire bool   `json:"obligatoire"`
}

type regleIA struct {
	ID        string `json:"id"`
	Categorie string `json:"categorie"`
	Titre     string `json:"titre"`
}

type commandeIA struct {
	Nom          string          `json:"nom"`
	Description  string          `json:"description"`
	Usage        string          `json:"usage"`
	Positionnels []positionnelIA `json:"positionnels,omitempty"`
	Options      []optionIA      `json:"options"`
	Regles       []regleIA       `json:"regles,omitempty"` // uniquement "check"
}

type manifesteIA struct {
	Outil         string            `json:"outil"`
	Version       string            `json:"version"`
	Description   string            `json:"description"`
	Auteur        string            `json:"auteur"`
	Licence       string            `json:"licence"`
	Source        string            `json:"source"`
	Usage         string            `json:"usage"`
	CodesSortie   map[string]string `json:"codes_sortie"`
	ConseilsAgent []string          `json:"conseils_agent"`
	Commandes     []commandeIA      `json:"commandes"`
}

// afficherManifesteIA imprime sur stdout la description complète de l'outil au
// format JSON, pour qu'un agent découvre programmatiquement toutes les commandes,
// leurs options et (pour check) le registre de règles — sans avoir à parser la
// sortie texte de --help.
func afficherManifesteIA() {
	m := manifesteIA{
		Outil:       "filiatium",
		Version:     version,
		Description: "Validation et correction de GEDCOM 5.5.1 (compatibilité Gramps)",
		Auteur:      "Nicolas Jalibert",
		Licence:     "MIT",
		Source:      "https://github.com/FamilyTree-nicoolaj/filiatium",
		Usage:       "filiatium <commande> [options]",
		CodesSortie: map[string]string{
			"0": "rien à signaler / succès",
			"1": "signalements présents, ou écriture refusée par l'auto-vérification",
			"2": "erreur d'usage ou d'entrée/sortie",
		},
		ConseilsAgent: []string{
			`Fournir toujours les options nécessaires (fichier, --nom, etc.) : aucune commande ne lit l'entrée standard si elles le sont — le mode guidé ("filiatium" sans argument) et l'assistant de "add" sont réservés à un usage humain en terminal.`,
			`Utiliser --json sur chaque commande pour une sortie strictement analysable plutôt que le texte destiné à un humain.`,
			`fix / add / apply : simulation par défaut, --write pour écrire ; toujours simuler d'abord et relire le résultat.`,
			`merge n'écrit jamais de GEDCOM : produire un plan avec --plan, le relire, puis l'exécuter avec "apply --write".`,
			`Les seuils de réalisme (check, catégorie realisme) sont réglables via un fichier "filiatium.json" posé à côté du GEDCOM.`,
		},
	}

	for _, c := range commandes {
		m.Commandes = append(m.Commandes, commandeIAPour(c))
	}
	sort.Slice(m.Commandes, func(i, j int) bool { return m.Commandes[i].Nom < m.Commandes[j].Nom })

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(m)
}

// commandeIAPour construit la description d'une commande en enregistrant ses
// options sur un FlagSet jetable (via le même flagsXxx que l'exécution réelle),
// puis en les listant par fs.VisitAll. Usage et positionnels, qui n'ont pas
// d'équivalent dans le paquet flag, restent déclarés ici.
func commandeIAPour(c Commande) commandeIA {
	ci := commandeIA{Nom: c.Nom, Description: c.Description}
	fs := flag.NewFlagSet(c.Nom, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	switch c.Nom {
	case "check":
		flagsCheck(fs)
		ci.Usage = "filiatium check <fichier.ged> [options]"
		ci.Positionnels = []positionnelIA{
			{Nom: "fichier.ged", Description: "chemin du GEDCOM à vérifier", Obligatoire: true},
		}
		for _, r := range rules.Registre {
			ci.Regles = append(ci.Regles, regleIA{ID: r.ID, Categorie: r.Categorie, Titre: r.Titre})
		}
	case "fix":
		flagsFix(fs)
		ci.Usage = "filiatium fix <fichier.ged> [options]"
		ci.Positionnels = []positionnelIA{
			{Nom: "fichier.ged", Description: "chemin du GEDCOM à corriger", Obligatoire: true},
		}
	case "add":
		flagsAdd(fs)
		ci.Usage = "filiatium add <fichier.ged> [options]"
		ci.Positionnels = []positionnelIA{
			{Nom: "fichier.ged", Description: "chemin du GEDCOM où ajouter l'individu", Obligatoire: true},
		}
	case "apply":
		flagsApply(fs)
		ci.Usage = "filiatium apply <correctif.json> [options]"
		ci.Positionnels = []positionnelIA{
			{Nom: "correctif.json", Description: `fichier de correctif déclaratif ; la cible .ged est indiquée dans son champ "cible"`, Obligatoire: true},
		}
	case "merge":
		flagsMerge(fs)
		ci.Usage = "filiatium merge --analyse <base.ged> <apport.ged> [options]"
		ci.Positionnels = []positionnelIA{
			{Nom: "base.ged", Description: "arbre de référence", Obligatoire: true},
			{Nom: "apport.ged", Description: "arbre à analyser en vue d'une fusion dans base.ged", Obligatoire: true},
		}
	}

	fs.VisitAll(func(fl *flag.Flag) {
		ci.Options = append(ci.Options, optionIA{
			Nom: fl.Name, Type: typeDeFlag(fl), Defaut: fl.DefValue, Description: fl.Usage,
		})
	})
	sort.Slice(ci.Options, func(i, j int) bool { return ci.Options[i].Nom < ci.Options[j].Nom })
	return ci
}

func typeDeFlag(fl *flag.Flag) string {
	if bv, ok := fl.Value.(interface{ IsBoolFlag() bool }); ok && bv.IsBoolFlag() {
		return "bool"
	}
	return "string"
}

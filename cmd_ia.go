package main

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"reflect"
	"sort"

	"github.com/FamilyTree-nicoolaj/filiatium/add"
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

// champJSON décrit un champ d'un fichier JSON accepté en entrée (ex. "add
// --fichier"). Construit par réflexion sur la struct Go réelle (voir
// schemaRequeteAjout) : l'existence et le type de chaque champ ne peuvent donc pas
// diverger de ce que le décodage JSON accepte vraiment — seule la description est
// écrite à la main, une prose que la réflexion ne peut pas extraire.
type champJSON struct {
	Nom         string `json:"nom"`
	Type        string `json:"type"`
	Obligatoire bool   `json:"obligatoire"`
	Description string `json:"description"`
}

// schemaJSON décrit le format d'un fichier JSON accepté en entrée par une commande.
type schemaJSON struct {
	Description string      `json:"description"`
	Forme       string      `json:"forme"`
	Champs      []champJSON `json:"champs"`
	Exemple     any         `json:"exemple"`
}

type commandeIA struct {
	Nom          string          `json:"nom"`
	Description  string          `json:"description"`
	Usage        string          `json:"usage"`
	Positionnels []positionnelIA `json:"positionnels,omitempty"`
	Options      []optionIA      `json:"options"`
	Regles       []regleIA       `json:"regles,omitempty"`       // uniquement "check"
	FichierJSON  *schemaJSON     `json:"fichier_json,omitempty"` // uniquement "add" (--fichier)
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
			`fix / add / apply / forcemerge / publish : simulation par défaut, --write pour écrire ; toujours simuler d'abord et relire le résultat.`,
			`automerge n'écrit jamais de GEDCOM : produire un plan avec --plan, le relire, puis l'exécuter avec "apply --write". Le plan déduplique déjà le contenu identique et complète les fiches appariées ; --fusionner règle jusqu'où (identiques|certaines|probables|tout, défaut certaines) — au-delà, un rapprochement reste visible au rapport sans jamais entrer dans le plan.`,
			`forcemerge fusionne directement deux GEDCOM vers un nouveau fichier dst.ged (jamais l'une des deux sources, qui restent intactes), à partir de paires d'individus "xrefA:xrefB" déclarées explicitement (mode miroir) — --fusionner règle jusqu'où l'appariement automatique complète ces ancres. Un conflit de valeur garde la valeur de srcA mais préserve celle de srcB en NOTE : rien n'est jamais perdu silencieusement.`,
			`publish retire, vers un nouveau fichier dst.ged (jamais src.ged), les faits datés qu'une règle de réalisme (R1-R13) juge invraisemblables et qu'aucune citation SOUR n'étaye — --niveau règle la prudence : strict (impossibilités sans seuil, défaut), modere (+ coïncidences suspectes), large (+ règles à seuil réglable). Un fait sourcé n'est jamais supprimé.`,
			`Les seuils de réalisme (check, catégorie realisme ; publish --niveau large) sont réglables via un fichier "filiatium.json" posé à côté du GEDCOM.`,
			`import construit un GEDCOM neuf à partir de captures de fiches Geneanet (OCR interne via tesseract, jamais visible pour l'appelant) et écrit toujours vers un nouveau dst.ged — dédupliquant automatiquement les personnes qui se recoupent entre plusieurs fiches. La garde d'écriture y ignore la catégorie realisme (des signalements comme R13 sont attendus sur des données réelles, pas un défaut de la construction) mais bloque toujours sur structure/liens/doublons.`,
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
		schema := schemaRequeteAjout()
		ci.FichierJSON = &schema
	case "apply":
		flagsApply(fs)
		ci.Usage = "filiatium apply <correctif.json> [options]"
		ci.Positionnels = []positionnelIA{
			{Nom: "correctif.json", Description: `fichier de correctif déclaratif ; la cible .ged est indiquée dans son champ "cible"`, Obligatoire: true},
		}
	case "automerge":
		flagsAutomerge(fs)
		ci.Usage = "filiatium automerge --analyse <base.ged> <apport.ged> [options]"
		ci.Positionnels = []positionnelIA{
			{Nom: "base.ged", Description: "arbre de référence", Obligatoire: true},
			{Nom: "apport.ged", Description: "arbre à analyser en vue d'une fusion dans base.ged", Obligatoire: true},
		}
	case "renumber":
		flagsRenumber(fs)
		ci.Usage = "filiatium renumber <fichier.ged> (--source <xref> | --decalage <n> | --prefixe <lettre>) [options]  OU  filiatium renumber --depuis-table <table.json> [options]"
		ci.Positionnels = []positionnelIA{
			{Nom: "fichier.ged", Description: "chemin du GEDCOM à renuméroter — requis avec --source/--decalage/--prefixe, absent en mode --depuis-table", Obligatoire: false},
		}
	case "forcemerge":
		flagsForcemerge(fs)
		ci.Usage = "filiatium forcemerge <dst.ged> <srcA.ged> <srcB.ged> <xrefA1:xrefB1> [xrefA2:xrefB2 ...] [options]"
		ci.Positionnels = []positionnelIA{
			{Nom: "dst.ged", Description: "fichier de sortie — jamais srcA.ged ni srcB.ged, qui restent tous deux intacts", Obligatoire: true},
			{Nom: "srcA.ged", Description: "premier arbre source", Obligatoire: true},
			{Nom: "srcB.ged", Description: "second arbre source", Obligatoire: true},
			{Nom: "xrefA:xrefB", Description: "au moins une paire d'ancres \"xref de srcA:xref de srcB\" désignant le même individu (mode miroir)", Obligatoire: true},
		}
	case "publish":
		flagsPublish(fs)
		ci.Usage = "filiatium publish <src.ged> <dst.ged> [options]"
		ci.Positionnels = []positionnelIA{
			{Nom: "src.ged", Description: "GEDCOM source, jamais modifié", Obligatoire: true},
			{Nom: "dst.ged", Description: "fichier de sortie, avec les faits non sourcés jugés invraisemblables retirés", Obligatoire: true},
		}
	case "import":
		flagsImport(fs)
		ci.Usage = "filiatium import <dst.ged> <fiche1.html> [fiche2.html ...] [options]"
		ci.Positionnels = []positionnelIA{
			{Nom: "dst.ged", Description: "fichier de sortie — toujours nouveau, jamais un fichier déjà existant", Obligatoire: true},
			{Nom: "fiche.html ...", Description: "code source HTML d'une ou plusieurs fiches individuelles Geneanet", Obligatoire: true},
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

// descriptionsChampsAjout donne le sens de chaque champ de add.Requete — la seule
// partie qu'une réflexion ne peut pas extraire (pas de commentaires à l'exécution).
// Si un champ est ajouté/retiré/renommé dans add.Requete, schemaRequeteAjout le
// répercute automatiquement (avec une description vide tant qu'elle n'est pas
// complétée ici) : rien ne peut rester silencieusement désynchronisé de la struct.
var descriptionsChampsAjout = map[string]string{
	"Nom":              `nom complet au format GEDCOM, ex. "Jean /Dupret/" — seul champ obligatoire`,
	"Sexe":             "M, F, ou vide si inconnu",
	"Naissance":        `date de naissance GEDCOM, ex. "12 MAR 1805", ou vide si inconnue`,
	"Deces":            "date de décès GEDCOM, ou vide si inconnue",
	"Pere":             "xref du père déjà présent dans le fichier, ou vide si aucun",
	"Mere":             "xref de la mère déjà présente dans le fichier, ou vide si aucune",
	"Conjoint":         "xref du conjoint déjà présent dans le fichier, ou vide si aucun",
	"Note":             "justification de l'ajout, écrite en 1 NOTE sur le nouvel individu",
	"IgnorerHomonymes": "passe outre un homonyme potentiel détecté (équivalent de --force)",
}

// schemaRequeteAjout décrit, par réflexion sur add.Requete (la struct réellement
// décodée par "add --fichier"), le format attendu : un objet pour un individu, ou
// un tableau d'objets pour en ajouter plusieurs en une fois. Voir add/add.go —
// aucune balise json (correspondance insensible à la casse par défaut), donc les
// noms de champs listés ici sont exactement ceux acceptés dans le fichier.
func schemaRequeteAjout() schemaJSON {
	t := reflect.TypeOf(add.Requete{})
	champs := make([]champJSON, 0, t.NumField())
	exempleUn := map[string]any{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		champs = append(champs, champJSON{
			Nom:         f.Name,
			Type:        f.Type.Kind().String(),
			Obligatoire: f.Name == "Nom",
			Description: descriptionsChampsAjout[f.Name],
		})
	}
	exempleUn = map[string]any{
		"Nom": "Jean /Dupret/", "Sexe": "M", "Naissance": "12 MAR 1805",
		"Pere": "I0123", "Mere": "I0124", "Conjoint": "I0200",
	}
	exempleDeux := map[string]any{"Nom": "Marie /Dupret/", "Sexe": "F", "Naissance": "3 JUL 1808", "Pere": "I0123", "Mere": "I0124"}

	return schemaJSON{
		Description: `Format du fichier passé à "add --fichier" : décrit un individu à ajouter, ou plusieurs en une seule fois.`,
		Forme:       `un objet (un seul individu) OU un tableau d'objets (plusieurs individus, ajoutés un par un dans l'ordre du tableau)`,
		Champs:      champs,
		Exemple: map[string]any{
			"un_seul_individu":    exempleUn,
			"plusieurs_individus": []any{exempleUn, exempleDeux},
		},
	}
}

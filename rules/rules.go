// Package rules regroupe les contrôles de conformité GEDCOM sous un registre unique —
// port fusionné de valider.py, controle.py, controle_liens.py et controle_doublons.py
// (~/Documents/Généalogie/outils/). Chaque règle est indépendante et rejouable seule ;
// `check` (commande CLI) les enchaîne toutes, ce qui manquait jusqu'ici.
package rules

import (
	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

// Seuils est un alias pratique : les règles n'ont pas à importer config directement.
type Seuils = config.Seuils

// Finding est un signalement : une incohérence à regarder, jamais la preuve d'une
// erreur (une date "1676" sans jour ni mois est souvent une estimation).
type Finding struct {
	Regle   string   `json:"regle"`           // "L1"
	Message string   `json:"message"`         // en français, même formulation que les scripts d'origine
	Xrefs   []string `json:"xrefs,omitempty"` // pour le tri, le filtrage et l'auto-vérification (add/merge)
}

// Regle est une vérification nommée, appelable seule ou via le Registre complet.
type Regle struct {
	ID, Titre, Categorie string
	Verifie              func(*gedcom.Gedcom, Seuils) []Finding
}

// Registre liste toutes les règles connues, dans l'ordre d'affichage de `check`.
var Registre = []Regle{
	{"S1", "ligne malformée", "structure", S1},
	{"S2", "saut de niveau", "structure", S2},
	{"S3", "ligne de plus de 255 caractères", "structure", S3},
	{"S4", "pointeur non résolu", "structure", S4},
	{"S5", "le fichier ne commence/finit pas par HEAD/TRLR", "structure", S5},

	{"L1", "FAM.HUSB/WIFE sans FAMS réciproque côté INDI", "liens", L1},
	{"L2", "FAM.CHIL sans FAMC réciproque côté INDI", "liens", L2},
	{"L3", "INDI.FAMS vers une FAM qui ne le porte pas", "liens", L3},
	{"L4", "INDI.FAMC vers une FAM qui ne le porte pas en CHIL", "liens", L4},
	{"L5", "pointeur vers un xref qui n'existe pas", "liens", L5},
	{"L6", "FAM incomplète (ni HUSB, ni WIFE, ni CHIL)", "liens", L6},

	{"D1", "FAM quasi-doublons (conjoint et enfant communs)", "doublons", D1},
	{"D2", "germains portés comme HUSB/WIFE d'une même FAM", "doublons", D2},
	{"D3", "FAMS répété sur un même individu", "doublons", D3},
	{"D4", "CHIL répété sur une même FAM", "doublons", D4},

	{"R1", "mariage recopié de celui des parents", "realisme", R1},
	{"R2", "mariage avant la naissance d'un époux, ou à moins de 13 ans", "realisme", R2},
	{"R3", "décès avant la naissance, ou longévité invraisemblable", "realisme", R3},
	{"R4", "enfant né après le décès d'un parent, ou avant ses 13 ans", "realisme", R4},
	{"R5", "deux personnes distinctes avec les mêmes dates de naissance et décès", "realisme", R5},
	{"R6", "écart d'âge invraisemblable entre époux", "realisme", R6},
	{"R7", "mère trop jeune ou trop âgée à la naissance d'un enfant", "realisme", R7},
	{"R8", "père trop âgé à la naissance d'un enfant", "realisme", R8},
	{"R9", "germains nés à moins de 9 mois d'intervalle", "realisme", R9},
	{"R10", "mariage postérieur au décès d'un époux", "realisme", R10},
	{"R11", "date dans le futur", "realisme", R11},
	{"R12", "ordre des événements incohérent (baptême/naissance, inhumation/décès)", "realisme", R12},
	{"R13", "aucun décès enregistré pour un individu manifestement trop âgé", "realisme", R13},
}

// etiq formate un xref comme dans les scripts d'origine : "I0250 Etienne Castel".
func etiq(g *gedcom.Gedcom, x string) string {
	if r, ok := g.Get(x); ok {
		return x + " " + r.Nom()
	}
	return x
}

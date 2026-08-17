// Package fix applique les corrections mécaniques sûres identifiées par le registre
// de règles : celles qui ne demandent aucun jugement, seulement de rétablir une
// information déjà déductible sans ambiguïté (liens réciproques, pointeurs répétés,
// lignes trop longues). Rien ici ne supprime d'information généalogique.
package fix

import (
	"fmt"
	"unicode/utf8"

	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/rules"
)

// Categorie identifie la classe de correction mécanique.
type Categorie string

const (
	LienReciproque   Categorie = "lien réciproque manquant"
	PointeurDuplique Categorie = "pointeur dupliqué"
	LigneTropLongue  Categorie = "ligne > 255 caractères"
)

// Correctif est une correction mécanique candidate. Appliquer() la réalise sur le
// *gedcom.Gedcom en mémoire — rien n'est jamais écrit sur disque ici, c'est au
// code appelant (cmd_fix.go) de décider s'il sauvegarde.
type Correctif struct {
	Categorie Categorie
	Xref      string // enregistrement concerné, pour affichage
	Diff      string // ex. "(I0614) ajouter `1 FAMS @F0259@`"

	appliquer func()
}

func (c Correctif) Appliquer() { c.appliquer() }

// Detecter recense toutes les corrections mécaniques applicables. S'appuie sur les
// mêmes règles que `check` (L1/L2/D3/D4) plutôt que de redétecter : la seule chose
// que fix ajoute, c'est comment agir sur chaque signalement.
func Detecter(g *gedcom.Gedcom) []Correctif {
	var out []Correctif
	out = append(out, detecterLiens(g)...)
	out = append(out, detecterDoublons(g)...)
	out = append(out, detecterLignesLongues(g)...)
	return out
}

func detecterLiens(g *gedcom.Gedcom) []Correctif {
	var out []Correctif
	seuils := config.Defauts() // L1/L2 n'utilisent aucun seuil, mais Verifie l'exige
	for _, f := range rules.L1(g, seuils) {
		fam, ind := f.Xrefs[0], f.Xrefs[1]
		out = append(out, Correctif{
			Categorie: LienReciproque, Xref: ind,
			Diff: fmt.Sprintf("(%s) ajouter `1 FAMS @%s@`", ind, fam),
			appliquer: func() {
				if r, ok := g.Get(ind); ok {
					r.AddFams(fam)
				}
			},
		})
	}
	for _, f := range rules.L2(g, seuils) {
		fam, ind := f.Xrefs[0], f.Xrefs[1]
		out = append(out, Correctif{
			Categorie: LienReciproque, Xref: ind,
			Diff: fmt.Sprintf("(%s) ajouter `1 FAMC @%s@`", ind, fam),
			appliquer: func() {
				if r, ok := g.Get(ind); ok {
					r.AddFamc(fam)
				}
			},
		})
	}
	return out
}

func detecterDoublons(g *gedcom.Gedcom) []Correctif {
	var out []Correctif
	seuils := config.Defauts()
	for _, f := range rules.D3(g, seuils) {
		ind, fam := f.Xrefs[0], f.Xrefs[1]
		ligne := "1 FAMS @" + fam + "@"
		out = append(out, Correctif{
			Categorie: PointeurDuplique, Xref: ind,
			Diff: fmt.Sprintf("(%s) retirer le doublon `%s`", ind, ligne),
			appliquer: func() {
				if r, ok := g.Get(ind); ok {
					r.SupprimerOccurrenceEnTrop(ligne)
				}
			},
		})
	}
	for _, f := range rules.D4(g, seuils) {
		fam, c := f.Xrefs[0], f.Xrefs[1]
		ligne := "1 CHIL @" + c + "@"
		out = append(out, Correctif{
			Categorie: PointeurDuplique, Xref: fam,
			Diff: fmt.Sprintf("(%s) retirer le doublon `%s`", fam, ligne),
			appliquer: func() {
				if r, ok := g.Get(fam); ok {
					r.SupprimerOccurrenceEnTrop(ligne)
				}
			},
		})
	}
	return out
}

// detecterLignesLongues ne passe pas par rules.S3 : ce contrôle ne garde qu'un
// aperçu tronqué à 60 caractères (suffisant pour un rapport, pas pour retrouver et
// remplacer la ligne exacte), donc un balayage dédié, plus direct qu'enrichir
// Finding pour un seul appelant.
func detecterLignesLongues(g *gedcom.Gedcom) []Correctif {
	var out []Correctif
	for _, r := range g.Records {
		record := r
		for _, ligne := range r.Lignes {
			if utf8.RuneCountInString(ligne) <= 255 {
				continue
			}
			if _, ok := gedcom.Decoupe(ligne); !ok {
				continue // ligne malformée : signalé par S1, hors périmètre de fix
			}
			ligneOriginale := ligne
			apercu := []rune(ligneOriginale)
			if len(apercu) > 60 {
				apercu = apercu[:60]
			}
			out = append(out, Correctif{
				Categorie: LigneTropLongue, Xref: record.Xref,
				Diff: fmt.Sprintf("(%s) replier une ligne de %d caractères en CONC : %s…",
					record.Xref, utf8.RuneCountInString(ligneOriginale), string(apercu)),
				appliquer: func() {
					record.ReplierLigne(ligneOriginale)
				},
			})
		}
	}
	return out
}

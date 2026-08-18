// Package publish repère, parmi les règles de réalisme (R1-R13), les faits datés
// qu'aucune source ne vient étayer et qu'une incohérence rend peu plausibles — puis
// les retire, en ne touchant jamais un fait sourcé. Ne modifie jamais le
// *gedcom.Gedcom qu'on lui passe tant qu'Appliquer n'est pas appelé explicitement :
// Calculer se contente de désigner les candidats.
package publish

import (
	"fmt"
	"sort"

	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/rules"
)

// Niveau borne quelles règles de réalisme désignent des candidats à la suppression —
// chaque cran est un sur-ensemble du précédent.
type Niveau int

const (
	// NiveauStrict ne retient que les impossibilités strictes, sans seuil réglable :
	// R10 (mariage postérieur au décès), R11 (date dans le futur), R12 (ordre
	// baptême/naissance ou inhumation/décès incohérent).
	NiveauStrict Niveau = iota
	// NiveauModere ajoute les coïncidences suspectes (copier-coller probable),
	// toujours sans seuil réglable : R1 (mariage identique à celui des parents), R5
	// (deux personnes aux mêmes dates de naissance/décès).
	NiveauModere
	// NiveauLarge ajoute les 8 règles restantes, toutes basées sur un seuil réglable
	// via filiatium.json (âge min/max parent, longévité, écarts...) : R2, R3, R4,
	// R6, R7, R8, R9, R13 (R13 ne désigne jamais rien de concret, voir rules.R13).
	NiveauLarge
)

func (n Niveau) String() string {
	switch n {
	case NiveauStrict:
		return "strict"
	case NiveauModere:
		return "modere"
	case NiveauLarge:
		return "large"
	default:
		return "?"
	}
}

// ParseNiveau décode l'option --niveau.
func ParseNiveau(s string) (Niveau, error) {
	switch s {
	case "strict":
		return NiveauStrict, nil
	case "modere":
		return NiveauModere, nil
	case "large":
		return NiveauLarge, nil
	default:
		return 0, fmt.Errorf("niveau de confiance inconnu : %q (attendu strict|modere|large)", s)
	}
}

var reglesParNiveau = map[Niveau]map[string]bool{
	NiveauStrict: ensemble("R10", "R11", "R12"),
	NiveauModere: ensemble("R10", "R11", "R12", "R1", "R5"),
	NiveauLarge:  ensemble("R1", "R2", "R3", "R4", "R5", "R6", "R7", "R8", "R9", "R10", "R11", "R12", "R13"),
}

func ensemble(ids ...string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// Candidat est un fait daté (BIRT, MARR, DEAT...) mis en cause par une règle de
// réalisme. Sourced indique qu'il est protégé par une citation SOUR (voir
// gedcom.Record.ASource) — Appliquer ne le touche alors jamais, quel que soit le
// niveau demandé, exactement comme l'exige `publish`.
type Candidat struct {
	Xref    string `json:"xref"`
	Tag     string `json:"tag"`
	Regle   string `json:"regle"`
	Message string `json:"message"`
	Sourced bool   `json:"sourced"`
}

// Calculer rejoue les règles de réalisme actives au niveau demandé et renvoie, pour
// chaque fait distinct qu'elles mettent en cause, un Candidat — dédupliqué (un même
// fait peut être visé par plusieurs règles à la fois) et dans un ordre déterministe
// (xref puis tag). Ne modifie jamais g.
func Calculer(g *gedcom.Gedcom, niveau Niveau, seuils config.Seuils) []Candidat {
	actives := reglesParNiveau[niveau]
	sourceCache := map[string]bool{}
	sourced := func(xref string) bool {
		if v, ok := sourceCache[xref]; ok {
			return v
		}
		rec, ok := g.Get(xref)
		v := ok && rec.ASource()
		sourceCache[xref] = v
		return v
	}

	vus := map[string]bool{}
	var out []Candidat
	for _, regle := range rules.Registre {
		if regle.Categorie != "realisme" || !actives[regle.ID] {
			continue
		}
		for _, f := range regle.Verifie(g, seuils) {
			for _, fait := range f.Faits {
				cle := fait.Xref + "|" + fait.Tag
				if vus[cle] {
					continue
				}
				rec, ok := g.Get(fait.Xref)
				if !ok || rec.Evenement(fait.Tag) == nil {
					continue // pointeur pendant, ou événement déjà absent
				}
				vus[cle] = true
				out = append(out, Candidat{
					Xref: fait.Xref, Tag: fait.Tag, Regle: f.Regle, Message: f.Message,
					Sourced: sourced(fait.Xref),
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Xref != out[j].Xref {
			return out[i].Xref < out[j].Xref
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}

// Appliquer retire, EN PLACE sur g, le bloc d'événement de chaque candidat non
// sourcé (Sourced==true est toujours ignoré, même transmis explicitement). Renvoie
// le nombre de faits effectivement supprimés.
func Appliquer(g *gedcom.Gedcom, candidats []Candidat) int {
	n := 0
	for _, c := range candidats {
		if c.Sourced {
			continue
		}
		if rec, ok := g.Get(c.Xref); ok && rec.SupprimerEvenement(c.Tag) {
			n++
		}
	}
	return n
}

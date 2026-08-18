// Règles de réalisme étendues (R7-R13), au-delà du périmètre de controle.py —
// chaque nouveau contrôle couvre une classe d'erreur distincte et réellement
// rencontrée en généalogie, avec des seuils réglables (voir config.Seuils).
package rules

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

var moisGedcom = map[string]int{
	"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
	"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
}

// dateExacte extrait (jour, mois, année) d'une date GEDCOM complète "D MON AAAA"
// (ex. "12 APR 2019"). ok=false pour toute date approximative ou partielle
// (ABT/BET...AND/EST, année seule) : la précision qu'exige R9 n'a alors aucun sens.
func dateExacte(dateGedcom string) (jour, mois, annee int, ok bool) {
	champs := strings.Fields(dateGedcom)
	if len(champs) != 3 {
		return 0, 0, 0, false
	}
	j, errJ := strconv.Atoi(champs[0])
	m, connu := moisGedcom[strings.ToUpper(champs[1])]
	a, errA := strconv.Atoi(champs[2])
	if errJ != nil || !connu || errA != nil {
		return 0, 0, 0, false
	}
	return j, m, a, true
}

// R7 signale une mère de moins de AgeMinMere ou de plus de AgeMaxMere ans à la
// naissance d'un enfant.
func R7(g *gedcom.Gedcom, s Seuils) []Finding {
	var out []Finding
	for _, fam := range g.Familles() {
		mere := fam.Valeur("WIFE")
		if mere == "" {
			continue
		}
		indMere, ok := g.Get(mere)
		if !ok {
			continue
		}
		mn, okMn := gedcom.Annee(indMere.Date("BIRT"))
		if !okMn {
			continue
		}
		for _, c := range fam.Valeurs("CHIL") {
			indC, ok := g.Get(c)
			if !ok {
				continue
			}
			cn, okCn := gedcom.Annee(indC.Date("BIRT"))
			if !okCn {
				continue
			}
			if age := cn - mn; age < s.AgeMinMere || age > s.AgeMaxMere {
				out = append(out, Finding{Regle: "R7",
					Message: fmt.Sprintf("%s : %s mère à %d ans (née en %d) de %s (né(e) en %d)",
						fam.Xref, etiq(g, mere), age, mn, etiq(g, c), cn),
					Xrefs: []string{fam.Xref, mere, c}, Faits: []Fait{{mere, "BIRT"}, {c, "BIRT"}}})
			}
		}
	}
	return out
}

// R8 signale un père de plus de AgeMaxPere ans à la naissance d'un enfant.
func R8(g *gedcom.Gedcom, s Seuils) []Finding {
	var out []Finding
	for _, fam := range g.Familles() {
		pere := fam.Valeur("HUSB")
		if pere == "" {
			continue
		}
		indPere, ok := g.Get(pere)
		if !ok {
			continue
		}
		pn, okPn := gedcom.Annee(indPere.Date("BIRT"))
		if !okPn {
			continue
		}
		for _, c := range fam.Valeurs("CHIL") {
			indC, ok := g.Get(c)
			if !ok {
				continue
			}
			cn, okCn := gedcom.Annee(indC.Date("BIRT"))
			if !okCn {
				continue
			}
			if age := cn - pn; age > s.AgeMaxPere {
				out = append(out, Finding{Regle: "R8",
					Message: fmt.Sprintf("%s : %s père à %d ans (né en %d) de %s (né(e) en %d)",
						fam.Xref, etiq(g, pere), age, pn, etiq(g, c), cn),
					Xrefs: []string{fam.Xref, pere, c}, Faits: []Fait{{pere, "BIRT"}, {c, "BIRT"}}})
			}
		}
	}
	return out
}

// R9 signale deux enfants d'une même famille nés à moins de EcartGermainsMoisMin
// mois d'intervalle — sauf date de naissance strictement identique (jumeaux,
// toléré). Ne considère que des dates complètes jour+mois+année : une date
// approximative n'a pas la précision nécessaire pour ce contrôle.
// ponytail : écart calculé en mois calendaires (année*12+mois), pas en jours exacts —
// une naissance le 1er et l'autre le 28 du même mois ressort à "0 mois d'écart"
// alors qu'ils sont proches de 4 semaines. Suffisant pour détecter les cas
// franchement impossibles (même année/mois) ; affiner en jours si ça manque de finesse.
func R9(g *gedcom.Gedcom, s Seuils) []Finding {
	type naissance struct {
		xref           string
		jour, mois, an int
	}
	var out []Finding
	for _, fam := range g.Familles() {
		var nes []naissance
		for _, c := range fam.Valeurs("CHIL") {
			indC, ok := g.Get(c)
			if !ok {
				continue
			}
			j, m, a, ok := dateExacte(indC.Date("BIRT"))
			if !ok {
				continue
			}
			nes = append(nes, naissance{c, j, m, a})
		}
		for i := 0; i < len(nes); i++ {
			for k := i + 1; k < len(nes); k++ {
				d1, d2 := nes[i], nes[k]
				if d1.an == d2.an && d1.mois == d2.mois && d1.jour == d2.jour {
					continue // même jour : jumeaux plausibles, toléré
				}
				ecart := abs((d1.an*12 + d1.mois) - (d2.an*12 + d2.mois))
				if ecart < s.EcartGermainsMoisMin {
					out = append(out, Finding{Regle: "R9",
						Message: fmt.Sprintf("%s : %s et %s nés à %d mois d'intervalle",
							fam.Xref, etiq(g, d1.xref), etiq(g, d2.xref), ecart),
						Xrefs: []string{fam.Xref, d1.xref, d2.xref},
						Faits: []Fait{{d1.xref, "BIRT"}, {d2.xref, "BIRT"}}})
				}
			}
		}
	}
	return out
}

// R10 signale un mariage postérieur au décès d'un des époux.
func R10(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, fam := range g.Familles() {
		am, okAm := gedcom.Annee(fam.Date("MARR"))
		if !okAm {
			continue
		}
		for _, ep := range []string{fam.Valeur("HUSB"), fam.Valeur("WIFE")} {
			if ep == "" {
				continue
			}
			indEp, ok := g.Get(ep)
			if !ok {
				continue
			}
			if dm, okDm := gedcom.Annee(indEp.Date("DEAT")); okDm && am > dm {
				out = append(out, Finding{Regle: "R10",
					Message: fmt.Sprintf("%s : mariage en %d, mais %s décédé(e) en %d",
						fam.Xref, am, etiq(g, ep), dm),
					Xrefs: []string{fam.Xref, ep}, Faits: []Fait{{fam.Xref, "MARR"}, {ep, "DEAT"}}})
			}
		}
	}
	return out
}

// R11 signale une date (BIRT/DEAT d'un individu, MARR d'une famille) postérieure à
// l'année en cours.
func R11(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	anneeCourante := time.Now().Year()
	for _, ind := range g.Individus() {
		for _, tag := range []string{"BIRT", "DEAT"} {
			if a, ok := gedcom.Annee(ind.Date(tag)); ok && a > anneeCourante {
				out = append(out, Finding{Regle: "R11",
					Message: fmt.Sprintf("%s : %s en %d, dans le futur", etiq(g, ind.Xref), tag, a),
					Xrefs:   []string{ind.Xref}, Faits: []Fait{{ind.Xref, tag}}})
			}
		}
	}
	for _, fam := range g.Familles() {
		if a, ok := gedcom.Annee(fam.Date("MARR")); ok && a > anneeCourante {
			out = append(out, Finding{Regle: "R11",
				Message: fmt.Sprintf("%s : MARR en %d, dans le futur", fam.Xref, a),
				Xrefs:   []string{fam.Xref}, Faits: []Fait{{fam.Xref, "MARR"}}})
		}
	}
	return out
}

// R12 signale un ordre d'événements incohérent sur un même individu : baptême après
// naissance, ou inhumation après décès.
func R12(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	paires := []struct{ tot, apres, motTot, motApres string }{
		{"BAPM", "BIRT", "baptême", "naissance"},
		{"BURI", "DEAT", "inhumation", "décès"},
	}
	for _, ind := range g.Individus() {
		for _, p := range paires {
			at, okAt := gedcom.Annee(ind.Date(p.tot))
			aa, okAa := gedcom.Annee(ind.Date(p.apres))
			if okAt && okAa && at < aa {
				out = append(out, Finding{Regle: "R12",
					Message: fmt.Sprintf("%s : %s en %d avant sa %s en %d",
						etiq(g, ind.Xref), p.motTot, at, p.motApres, aa),
					Xrefs: []string{ind.Xref}, Faits: []Fait{{ind.Xref, p.tot}, {ind.Xref, p.apres}}})
			}
		}
	}
	return out
}

// R13 signale un individu sans DEAT (ni date, ni marqueur "DEAT Y") né il y a plus
// de LongeviteMax ans — décès probablement non saisi plutôt que personne encore en vie.
func R13(g *gedcom.Gedcom, s Seuils) []Finding {
	var out []Finding
	anneeCourante := time.Now().Year()
	for _, ind := range g.Individus() {
		if ind.Evenement("DEAT") != nil {
			continue // décès déjà connu, même sans date (ex. "1 DEAT Y")
		}
		n, ok := gedcom.Annee(ind.Date("BIRT"))
		if !ok {
			continue
		}
		if age := anneeCourante - n; age > s.LongeviteMax {
			// Pas de Faits : R13 signale une ABSENCE de décès, rien de concret à
			// désigner pour publish (on ne supprime pas ce qui n'existe pas).
			out = append(out, Finding{Regle: "R13",
				Message: fmt.Sprintf("%s : né(e) en %d (%d ans), aucun décès enregistré",
					etiq(g, ind.Xref), n, age),
				Xrefs: []string{ind.Xref}})
		}
	}
	return out
}

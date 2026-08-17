// Port de controle.py (tests A-F) — cohérence généalogique : aucun de ces tests ne
// prouve une erreur, chacun signale une incohérence à regarder. Une date "1676" sans
// jour ni mois est souvent une estimation, et une estimation fausse de trois ans ne
// veut pas dire que la filiation est fausse.
package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

// R1 signale un mariage à la date identique à celui des parents d'un des époux —
// copier-coller probable.
func R1(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, fam := range g.Familles() {
		d := fam.Date("MARR")
		if d == "" {
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
			for _, fp := range indEp.FamcPedi() {
				pf, ok := g.Get(fp.Fam)
				if !ok {
					continue
				}
				if pf.Date("MARR") == d {
					out = append(out, Finding{Regle: "R1",
						Message: fmt.Sprintf("%s : mariage «%s» identique à celui des parents de %s (%s) — copier-coller probable",
							fam.Xref, d, etiq(g, ep), fp.Fam),
						Xrefs: []string{fam.Xref, ep, fp.Fam}})
				}
			}
		}
	}
	return out
}

// R2 signale un mariage avant la naissance d'un époux, ou à moins de AgeMinParent ans.
func R2(g *gedcom.Gedcom, s Seuils) []Finding {
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
			an, okAn := gedcom.Annee(indEp.Date("BIRT"))
			if !okAn {
				continue
			}
			switch {
			case am < an:
				out = append(out, Finding{Regle: "R2",
					Message: fmt.Sprintf("%s : mariage en %d, mais %s est né(e) en %d — %d an(s) avant sa naissance",
						fam.Xref, am, etiq(g, ep), an, an-am),
					Xrefs: []string{fam.Xref, ep}})
			case am-an < s.AgeMinParent:
				out = append(out, Finding{Regle: "R2",
					Message: fmt.Sprintf("%s : %s marié(e) à %d ans", fam.Xref, etiq(g, ep), am-an),
					Xrefs:   []string{fam.Xref, ep}})
			}
		}
	}
	return out
}

// R3 signale un décès avant la naissance, ou une longévité au-delà de LongeviteMax.
func R3(g *gedcom.Gedcom, s Seuils) []Finding {
	var out []Finding
	for _, ind := range g.Individus() {
		n, okN := gedcom.Annee(ind.Date("BIRT"))
		m, okM := gedcom.Annee(ind.Date("DEAT"))
		if !okN || !okM {
			continue
		}
		switch {
		case m < n:
			out = append(out, Finding{Regle: "R3",
				Message: fmt.Sprintf("%s : décès en %d avant la naissance en %d", etiq(g, ind.Xref), m, n),
				Xrefs:   []string{ind.Xref}})
		case m-n > s.LongeviteMax:
			out = append(out, Finding{Regle: "R3",
				Message: fmt.Sprintf("%s : %d ans (%d-%d)", etiq(g, ind.Xref), m-n, n, m),
				Xrefs:   []string{ind.Xref}})
		}
	}
	return out
}

// R4 signale un enfant né hors de la vie d'un parent : après le décès du père (avec
// une marge d'un an pour un enfant posthume ; aucune pour la mère), ou avant les
// AgeMinParent ans du parent.
func R4(g *gedcom.Gedcom, s Seuils) []Finding {
	var out []Finding
	type roleParent struct{ role, parent string }
	for _, fam := range g.Familles() {
		for _, rp := range []roleParent{{"père", fam.Valeur("HUSB")}, {"mère", fam.Valeur("WIFE")}} {
			if rp.parent == "" {
				continue
			}
			indParent, ok := g.Get(rp.parent)
			if !ok {
				continue
			}
			pn, okPn := gedcom.Annee(indParent.Date("BIRT"))
			pm, okPm := gedcom.Annee(indParent.Date("DEAT"))
			marge := 0
			if rp.role == "père" {
				marge = 1
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
				if okPm && cn > pm+marge {
					out = append(out, Finding{Regle: "R4",
						Message: fmt.Sprintf("%s : %s né(e) en %d, après le décès de son %s %s en %d",
							fam.Xref, etiq(g, c), cn, rp.role, etiq(g, rp.parent), pm),
						Xrefs: []string{fam.Xref, c, rp.parent}})
				}
				if okPn && cn-pn < s.AgeMinParent {
					out = append(out, Finding{Regle: "R4",
						Message: fmt.Sprintf("%s : %s né(e) en %d, %s %s né(e) en %d — %d an(s) d'écart",
							fam.Xref, etiq(g, c), cn, rp.role, etiq(g, rp.parent), pn, cn-pn),
						Xrefs: []string{fam.Xref, c, rp.parent}})
				}
			}
		}
	}
	return out
}

// R5 signale l'autre forme du copier-coller : un parent et son enfant qui partagent
// une date à l'identique, ou deux personnes quelconques avec les deux mêmes dates
// (naissance ET décès).
func R5(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, fam := range g.Familles() {
		for _, parent := range []string{fam.Valeur("HUSB"), fam.Valeur("WIFE")} {
			if parent == "" {
				continue
			}
			indParent, ok := g.Get(parent)
			if !ok {
				continue
			}
			for _, c := range fam.Valeurs("CHIL") {
				indC, ok := g.Get(c)
				if !ok {
					continue
				}
				for _, tm := range []struct{ tag, mot string }{{"BIRT", "naissance"}, {"DEAT", "décès"}} {
					a, b := indParent.Date(tm.tag), indC.Date(tm.tag)
					if a != "" && a == b {
						out = append(out, Finding{Regle: "R5",
							Message: fmt.Sprintf("%s : même date de %s «%s» pour %s et son enfant %s",
								fam.Xref, tm.mot, a, etiq(g, parent), etiq(g, c)),
							Xrefs: []string{fam.Xref, parent, c}})
					}
				}
			}
		}
	}

	type cle struct{ n, m string }
	vus := map[cle][]string{}
	for _, ind := range g.Individus() {
		n, m := ind.Date("BIRT"), ind.Date("DEAT")
		if n != "" && m != "" {
			k := cle{n, m}
			vus[k] = append(vus[k], ind.Xref)
		}
	}
	var cles []cle
	for k := range vus {
		cles = append(cles, k)
	}
	sort.Slice(cles, func(i, j int) bool {
		if cles[i].n != cles[j].n {
			return cles[i].n < cles[j].n
		}
		return cles[i].m < cles[j].m
	})
	for _, k := range cles {
		gens := vus[k]
		if len(gens) <= 1 {
			continue
		}
		sort.Strings(gens)
		var etiqs []string
		for _, x := range gens {
			etiqs = append(etiqs, etiq(g, x))
		}
		out = append(out, Finding{Regle: "R5",
			Message: fmt.Sprintf("mêmes dates (%s † %s) : %s", k.n, k.m, strings.Join(etiqs, ", ")),
			Xrefs:   gens})
	}
	return out
}

// R6 signale un écart d'âge entre époux au-delà de EcartEpouxMax.
func R6(g *gedcom.Gedcom, s Seuils) []Finding {
	var out []Finding
	for _, fam := range g.Familles() {
		h, w := fam.Valeur("HUSB"), fam.Valeur("WIFE")
		if h == "" || w == "" {
			continue
		}
		indH, okH := g.Get(h)
		indW, okW := g.Get(w)
		if !okH || !okW {
			continue
		}
		a, okA := gedcom.Annee(indH.Date("BIRT"))
		b, okB := gedcom.Annee(indW.Date("BIRT"))
		if !okA || !okB {
			continue
		}
		if ecart := abs(a - b); ecart > s.EcartEpouxMax {
			out = append(out, Finding{Regle: "R6",
				Message: fmt.Sprintf("%s : %d ans d'écart entre %s (%d) et %s (%d)",
					fam.Xref, ecart, etiq(g, h), a, etiq(g, w), b),
				Xrefs: []string{fam.Xref, h, w}})
		}
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

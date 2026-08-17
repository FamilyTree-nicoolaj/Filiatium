// Port de controle_doublons.py — doublons structurels qui font apparaître une même
// personne plusieurs fois dans un lecteur GEDCOM : des pointeurs surnuméraires,
// présents et cohérents dans les deux sens, mais qui créent un second chemin vers le
// même individu.
package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

// D1 signale deux FAM quasi-doublons : un conjoint commun et au moins un enfant
// commun, sans être des enregistrements strictement identiques.
func D1(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	familles := g.Familles()
	for i := 0; i < len(familles); i++ {
		for j := i + 1; j < len(familles); j++ {
			f1, f2 := familles[i], familles[j]
			h1, w1 := f1.Valeur("HUSB"), f1.Valeur("WIFE")
			h2, w2 := f2.Valeur("HUSB"), f2.Valeur("WIFE")
			conjointCommun := (h1 != "" && h1 == h2) || (w1 != "" && w1 == w2)
			if !conjointCommun {
				continue
			}
			communs := intersectionTriee(f1.Valeurs("CHIL"), f2.Valeurs("CHIL"))
			if len(communs) == 0 {
				continue
			}
			var etiqs []string
			for _, c := range communs {
				etiqs = append(etiqs, etiq(g, c))
			}
			out = append(out, Finding{Regle: "D1",
				Message: fmt.Sprintf("%s et %s : conjoint commun et enfant(s) commun(s) (%s) — probable doublon de famille",
					f1.Xref, f2.Xref, strings.Join(etiqs, ", ")),
				Xrefs: append([]string{f1.Xref, f2.Xref}, communs...)})
		}
	}
	return out
}

// D2 signale deux germains (même FAMC) portés comme HUSB/WIFE d'une même FAM —
// union consanguine apparente, souvent un doublon de personne plutôt qu'un fait réel.
func D2(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, fam := range g.Familles() {
		husb, wife := fam.Valeur("HUSB"), fam.Valeur("WIFE")
		if husb == "" || wife == "" {
			continue
		}
		rh, okH := g.Get(husb)
		rw, okW := g.Get(wife)
		if !okH || !okW {
			continue
		}
		famcH := map[string]bool{}
		for _, fp := range rh.FamcPedi() {
			famcH[fp.Fam] = true
		}
		var commun []string
		for _, fp := range rw.FamcPedi() {
			if famcH[fp.Fam] {
				commun = append(commun, fp.Fam)
			}
		}
		if len(commun) == 0 {
			continue
		}
		sort.Strings(commun)
		out = append(out, Finding{Regle: "D2",
			Message: fmt.Sprintf("%s : %s et %s sont tous deux enfants de %s — union entre germains apparente",
				fam.Xref, etiq(g, husb), etiq(g, wife), strings.Join(commun, ", ")),
			Xrefs: append([]string{fam.Xref, husb, wife}, commun...)})
	}
	return out
}

// D3 signale un même individu portant deux fois le pointeur "1 FAMS" vers la même FAM.
func D3(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, ind := range g.Individus() {
		vus := map[string]bool{}
		for _, f := range ind.Valeurs("FAMS") {
			if vus[f] {
				out = append(out, Finding{Regle: "D3",
					Message: fmt.Sprintf("%s : `1 FAMS @%s@` répété", etiq(g, ind.Xref), f),
					Xrefs:   []string{ind.Xref, f}})
			}
			vus[f] = true
		}
	}
	return out
}

// D4 signale une même FAM portant deux fois le pointeur "1 CHIL" vers le même individu.
func D4(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, fam := range g.Familles() {
		vus := map[string]bool{}
		for _, c := range fam.Valeurs("CHIL") {
			if vus[c] {
				out = append(out, Finding{Regle: "D4",
					Message: fmt.Sprintf("%s : `1 CHIL @%s@` répété", fam.Xref, c),
					Xrefs:   []string{fam.Xref, c}})
			}
			vus[c] = true
		}
	}
	return out
}

func intersectionTriee(a, b []string) []string {
	bset := map[string]bool{}
	for _, x := range b {
		bset[x] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, x := range a {
		if bset[x] && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

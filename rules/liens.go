// Port de controle_liens.py — cohérence des liens structurels INDI <-> FAM : aucune
// date n'est regardée ici, seulement les pointeurs FAMC/FAMS côté INDI et
// HUSB/WIFE/CHIL côté FAM. Un GEDCOM correct les porte dans les deux sens ; un sens
// manquant fait "disparaître" une branche dans un lecteur qui part de l'individu.
package rules

import (
	"fmt"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

// L1 signale un FAM.HUSB/WIFE sans FAMS réciproque côté INDI.
func L1(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, fam := range g.Familles() {
		for _, role := range []string{"HUSB", "WIFE"} {
			x := fam.Valeur(role)
			if x == "" {
				continue
			}
			ind, ok := g.Get(x)
			if !ok {
				continue // -> L5
			}
			if !contient(ind.Valeurs("FAMS"), fam.Xref) {
				out = append(out, Finding{Regle: "L1",
					Message: fmt.Sprintf("%s : %s = %s, mais %s n'a pas de `1 FAMS @%s@`",
						fam.Xref, role, etiq(g, x), x, fam.Xref),
					Xrefs: []string{fam.Xref, x}})
			}
		}
	}
	return out
}

// L2 signale un FAM.CHIL sans FAMC réciproque côté INDI.
func L2(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, fam := range g.Familles() {
		for _, c := range fam.Valeurs("CHIL") {
			ind, ok := g.Get(c)
			if !ok {
				continue // -> L5
			}
			a := false
			for _, fp := range ind.FamcPedi() {
				if fp.Fam == fam.Xref {
					a = true
					break
				}
			}
			if !a {
				out = append(out, Finding{Regle: "L2",
					Message: fmt.Sprintf("%s : CHIL = %s, mais %s n'a pas de `1 FAMC @%s@`",
						fam.Xref, etiq(g, c), c, fam.Xref),
					Xrefs: []string{fam.Xref, c}})
			}
		}
	}
	return out
}

// L3 signale un INDI.FAMS vers une FAM qui ne le porte pas en HUSB/WIFE.
func L3(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, ind := range g.Individus() {
		for _, f := range ind.Valeurs("FAMS") {
			fam, ok := g.Get(f)
			if !ok {
				continue // -> L5
			}
			if ind.Xref != fam.Valeur("HUSB") && ind.Xref != fam.Valeur("WIFE") {
				out = append(out, Finding{Regle: "L3",
					Message: fmt.Sprintf("%s : FAMS @%s@, mais %s ne le/la porte ni en HUSB ni en WIFE",
						etiq(g, ind.Xref), f, f),
					Xrefs: []string{ind.Xref, f}})
			}
		}
	}
	return out
}

// L4 signale un INDI.FAMC vers une FAM qui ne le porte pas en CHIL.
func L4(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, ind := range g.Individus() {
		for _, fp := range ind.FamcPedi() {
			fam, ok := g.Get(fp.Fam)
			if !ok {
				continue // -> L5
			}
			if !contient(fam.Valeurs("CHIL"), ind.Xref) {
				out = append(out, Finding{Regle: "L4",
					Message: fmt.Sprintf("%s : FAMC @%s@, mais %s ne le/la porte pas en CHIL",
						etiq(g, ind.Xref), fp.Fam, fp.Fam),
					Xrefs: []string{ind.Xref, fp.Fam}})
			}
		}
	}
	return out
}

// L5 signale un pointeur de parenté (FAMC/FAMS/HUSB/WIFE/CHIL) vers un xref inexistant.
func L5(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, ind := range g.Individus() {
		for _, f := range ind.Valeurs("FAMS") {
			if !g.Contains(f) {
				out = append(out, Finding{Regle: "L5",
					Message: fmt.Sprintf("%s : FAMS @%s@ — @%s@ n'existe pas", etiq(g, ind.Xref), f, f),
					Xrefs:   []string{ind.Xref, f}})
			}
		}
		for _, fp := range ind.FamcPedi() {
			if !g.Contains(fp.Fam) {
				out = append(out, Finding{Regle: "L5",
					Message: fmt.Sprintf("%s : FAMC @%s@ — @%s@ n'existe pas", etiq(g, ind.Xref), fp.Fam, fp.Fam),
					Xrefs:   []string{ind.Xref, fp.Fam}})
			}
		}
	}
	for _, fam := range g.Familles() {
		for _, role := range []string{"HUSB", "WIFE"} {
			x := fam.Valeur(role)
			if x != "" && !g.Contains(x) {
				out = append(out, Finding{Regle: "L5",
					Message: fmt.Sprintf("%s : %s @%s@ — @%s@ n'existe pas", fam.Xref, role, x, x),
					Xrefs:   []string{fam.Xref, x}})
			}
		}
		for _, c := range fam.Valeurs("CHIL") {
			if !g.Contains(c) {
				out = append(out, Finding{Regle: "L5",
					Message: fmt.Sprintf("%s : CHIL @%s@ — @%s@ n'existe pas", fam.Xref, c, c),
					Xrefs:   []string{fam.Xref, c}})
			}
		}
	}
	return out
}

// L6 signale une FAM fantôme : ni HUSB, ni WIFE, ni CHIL.
func L6(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, fam := range g.Familles() {
		if fam.Valeur("HUSB") == "" && fam.Valeur("WIFE") == "" && len(fam.Valeurs("CHIL")) == 0 {
			out = append(out, Finding{Regle: "L6",
				Message: fmt.Sprintf("%s : ni HUSB, ni WIFE, ni CHIL", fam.Xref), Xrefs: []string{fam.Xref}})
		}
	}
	return out
}

func contient(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

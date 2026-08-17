package gedcom

import (
	"fmt"
	"strings"
)

// Parents renvoie une entrée (pere, mere) par famille FAMC de xref. pedi="birth" ne
// retient que la filiation biologique — mais avec repli : si aucune FAMC n'est
// déclarée "birth", toutes sont rendues. Sans ce repli, un individu dont l'unique
// filiation est adoptive n'aurait plus d'ascendance du tout.
func (g *Gedcom) Parents(xref, pedi string) ([][2]string, error) {
	rec, ok := g.Get(xref)
	if !ok {
		return nil, fmt.Errorf("@%s@ n'existe pas", xref)
	}
	familles := rec.FamcPedi()
	if pedi != "" {
		var filtrees []FamcPedi
		for _, fp := range familles {
			if fp.Pedi == pedi {
				filtrees = append(filtrees, fp)
			}
		}
		if len(filtrees) > 0 {
			familles = filtrees
		}
	}
	var out [][2]string
	for _, fp := range familles {
		fam, ok := g.Get(fp.Fam)
		if !ok {
			return nil, fmt.Errorf("@%s@ n'existe pas", fp.Fam)
		}
		out = append(out, [2]string{fam.Valeur("HUSB"), fam.Valeur("WIFE")})
	}
	return out, nil
}

// Enfants renvoie les xref des enfants de toutes les familles où xref est conjoint.
func (g *Gedcom) Enfants(xref string) ([]string, error) {
	rec, ok := g.Get(xref)
	if !ok {
		return nil, fmt.Errorf("@%s@ n'existe pas", xref)
	}
	var out []string
	for _, f := range rec.Valeurs("FAMS") {
		fam, ok := g.Get(f)
		if !ok {
			return nil, fmt.Errorf("@%s@ n'existe pas", f)
		}
		out = append(out, fam.Valeurs("CHIL")...)
	}
	return out, nil
}

// Conjoint associe un partenaire (Xref) et la famille (Fam) qui les unit.
type Conjoint struct {
	Xref string
	Fam  string
}

// Conjoints renvoie les partenaires de xref, un par famille FAMS.
func (g *Gedcom) Conjoints(xref string) ([]Conjoint, error) {
	rec, ok := g.Get(xref)
	if !ok {
		return nil, fmt.Errorf("@%s@ n'existe pas", xref)
	}
	xrefNu := strings.Trim(xref, "@")
	var out []Conjoint
	for _, f := range rec.Valeurs("FAMS") {
		fam, ok := g.Get(f)
		if !ok {
			return nil, fmt.Errorf("@%s@ n'existe pas", f)
		}
		for _, p := range []string{fam.Valeur("HUSB"), fam.Valeur("WIFE")} {
			if p != "" && p != xrefNu {
				out = append(out, Conjoint{Xref: p, Fam: f})
			}
		}
	}
	return out, nil
}

// Sosa renvoie {xref: numéro sosa} en partant de racine (= sosa 1), en remontant par
// la filiation biologique quand elle est déclarée (voir Parents). Un xref inconnu ou
// une FAMC pendante interrompt simplement cette branche de l'ascendance plutôt que
// d'annuler tout le calcul — utile pour Show() sur un arbre encore troué.
// ponytail : silencieux sur les pointeurs pendants ; check/L5 les signale par ailleurs.
func (g *Gedcom) Sosa(racine string) map[string]int {
	type paire struct {
		xref string
		s    int
	}
	num := map[string]int{}
	file := []paire{{strings.Trim(racine, "@"), 1}}
	for len(file) > 0 {
		p := file[0]
		file = file[1:]
		if _, vu := num[p.xref]; vu {
			continue
		}
		num[p.xref] = p.s
		parents, err := g.Parents(p.xref, "birth")
		if err != nil {
			continue
		}
		for _, pm := range parents {
			if pere := pm[0]; pere != "" {
				file = append(file, paire{pere, 2 * p.s})
			}
			if mere := pm[1]; mere != "" {
				file = append(file, paire{mere, 2*p.s + 1})
			}
		}
	}
	return num
}

// Port de valider.py — contrôle structurel : le fichier reste un GEDCOM valide,
// indépendamment de toute question de justesse généalogique.
package rules

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

// limiteLigneGedcom551 est en caractères (runes), pas en octets — voir README :
// family.ged a des lignes UTF-8 accentuées qui dépassent 255 octets sans
// dépasser 255 caractères ; compter des octets y produirait des faux positifs.
const limiteLigneGedcom551 = 255

// S1 signale une ligne qui ne respecte pas la grammaire "n [@xref@] TAG [valeur]".
func S1(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, r := range g.Records {
		for _, l := range r.Lignes {
			if _, ok := gedcom.Decoupe(l); !ok {
				out = append(out, Finding{Regle: "S1", Message: fmt.Sprintf("ligne malformée : %q", l)})
			}
		}
	}
	return out
}

// S2 signale un saut de niveau invalide (ex. "0 X" suivi de "2 Y" sans "1" entre les
// deux). Ne considère que les lignes bien formées : une ligne malformée est du
// ressort de S1 et n'avance pas le niveau courant.
func S2(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	precedent := -1
	for _, r := range g.Records {
		for _, l := range r.Lignes {
			d, ok := gedcom.Decoupe(l)
			if !ok {
				continue
			}
			if d.Niveau > precedent+1 {
				out = append(out, Finding{Regle: "S2",
					Message: fmt.Sprintf("saut de niveau (%d -> %d) : %q", precedent, d.Niveau, l)})
			}
			precedent = d.Niveau
		}
	}
	return out
}

// S3 signale une ligne de plus de 255 caractères (GEDCOM 5.5.1).
func S3(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	for _, r := range g.Records {
		for _, l := range r.Lignes {
			n := utf8.RuneCountInString(l)
			if n <= limiteLigneGedcom551 {
				continue
			}
			apercu := []rune(l)
			if len(apercu) > 60 {
				apercu = apercu[:60]
			}
			out = append(out, Finding{Regle: "S3",
				Message: fmt.Sprintf("ligne de %d caractères (> %d) : %s…", n, limiteLigneGedcom551, string(apercu))})
		}
	}
	return out
}

// S4 signale un pointeur "@xref@" (à n'importe quel niveau > 0, sur n'importe quel
// tag) qui désigne un enregistrement inexistant. Plus large que L5, qui ne regarde
// que les pointeurs de parenté FAMC/FAMS/HUSB/WIFE/CHIL.
func S4(g *gedcom.Gedcom, _ Seuils) []Finding {
	definis := g.ParXref()
	vus := map[string]bool{}
	var out []Finding
	for _, r := range g.Records {
		for _, l := range r.Lignes {
			d, ok := gedcom.Decoupe(l)
			if !ok || d.Niveau == 0 {
				continue
			}
			if !strings.HasPrefix(d.Valeur, "@") || !strings.HasSuffix(d.Valeur, "@") {
				continue
			}
			x := strings.Trim(d.Valeur, "@")
			if _, def := definis[x]; def || vus[x] {
				continue
			}
			vus[x] = true
			out = append(out, Finding{Regle: "S4",
				Message: fmt.Sprintf("pointeur non résolu : @%s@", x), Xrefs: []string{x}})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Xrefs[0] < out[j].Xrefs[0] })
	return out
}

// S5 signale un fichier qui ne commence pas par "0 HEAD" ou ne finit pas par "0 TRLR".
func S5(g *gedcom.Gedcom, _ Seuils) []Finding {
	var out []Finding
	if len(g.Records) == 0 || g.Records[len(g.Records)-1].Tag != "TRLR" {
		out = append(out, Finding{Regle: "S5", Message: "le fichier ne se termine pas par 0 TRLR"})
	}
	if len(g.Records) == 0 || g.Records[0].Tag != "HEAD" {
		out = append(out, Finding{Regle: "S5", Message: "le fichier ne commence pas par 0 HEAD"})
	}
	return out
}

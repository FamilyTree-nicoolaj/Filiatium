package gedcom

import "regexp"

var pointeurXrefRe = regexp.MustCompile(`@([A-Za-z0-9_]+)@`)

// TraduireXrefs réécrit chaque pointeur "@XREF@" de lignes selon table ; un xref
// absent de table reste inchangé. Utilisé aussi bien pour renuméroter un fichier
// (voir le package renumber) que pour traduire les xrefs d'un apport vers ceux
// d'une base (voir le package merge).
func TraduireXrefs(lignes []string, table map[string]string) []string {
	out := make([]string, len(lignes))
	for i, l := range lignes {
		out[i] = pointeurXrefRe.ReplaceAllStringFunc(l, func(m string) string {
			x := m[1 : len(m)-1]
			if nv, ok := table[x]; ok {
				return "@" + nv + "@"
			}
			return m
		})
	}
	return out
}

// Retraduire renvoie une COPIE de g dont tous les pointeurs (y compris la ligne 0
// de chaque enregistrement, donc son propre xref) sont réécrits selon table ; g
// n'est jamais modifié. table nil ou vide produit une copie inchangée. Pour les
// simulations qui ne doivent pas toucher l'original (voir merge.Analyser).
func (g *Gedcom) Retraduire(table map[string]string) *Gedcom {
	records := make([]*Record, len(g.Records))
	for i, r := range g.Records {
		records[i] = nouveauRecord(TraduireXrefs(r.Lignes, table))
	}
	return &Gedcom{Records: records}
}

// Renumeroter réécrit EN PLACE tous les pointeurs de g selon table (y compris la
// ligne 0 de chaque enregistrement) — Chemin/bom/crlf/empreinteChargement restent
// ceux de g, pour que Save("") écrive au bon endroit avec la détection de conflit
// concurrent intacte. table nil ou vide laisse g inchangé. Renvoie g pour chaînage.
func (g *Gedcom) Renumeroter(table map[string]string) *Gedcom {
	for i, r := range g.Records {
		g.Records[i] = nouveauRecord(TraduireXrefs(r.Lignes, table))
	}
	return g
}

package gedcom

import (
	"fmt"
	"strings"
	"time"
)

// inserer insère v dans s à l'index pos, en décalant le reste.
func inserer(s []string, pos int, v string) []string {
	out := make([]string, 0, len(s)+1)
	out = append(out, s[:pos]...)
	out = append(out, v)
	out = append(out, s[pos:]...)
	return out
}

// SetEventDate remplace la "2 DATE" de l'événement tag, ou l'ajoute juste après
// "1 TAG". L'événement doit exister. Renvoie l'ancienne valeur ("" si absente).
func (r *Record) SetEventDate(tag, valeur string) (ancienne string, err error) {
	ev := r.Evenement(tag)
	if ev == nil {
		return "", fmt.Errorf("%s: pas d'événement %s", r.Xref, tag)
	}
	for j := ev.debut + 1; j < ev.fin; j++ {
		if l, ok := Decoupe(r.Lignes[j]); ok && l.Niveau == 2 && l.Tag == "DATE" {
			ancienne = l.Valeur
			r.Lignes[j] = "2 DATE " + valeur
			return ancienne, nil
		}
	}
	r.Lignes = inserer(r.Lignes, ev.debut+1, "2 DATE "+valeur)
	return "", nil
}

// AddCitation ajoute "1 SOUR @X@" (individu/famille, si event=="") ou "2 SOUR @X@"
// dans l'événement désigné. Ne fait rien si la citation est déjà là. Renvoie true
// si ajoutée ; erreur si event est demandé mais n'existe pas sur ce record.
func (r *Record) AddCitation(sourceXref, event string) (bool, error) {
	ptr := "@" + strings.Trim(sourceXref, "@") + "@"
	if event != "" {
		ev := r.Evenement(event)
		if ev == nil {
			return false, fmt.Errorf("%s: pas d'événement %s", r.Xref, event)
		}
		for _, ligne := range r.Lignes[ev.debut:ev.fin] {
			if ligne == "2 SOUR "+ptr {
				return false, nil
			}
		}
		r.Lignes = inserer(r.Lignes, ev.fin, "2 SOUR "+ptr)
		return true, nil
	}
	for _, ligne := range r.Lignes {
		if ligne == "1 SOUR "+ptr {
			return false, nil
		}
	}
	r.Lignes = inserer(r.Lignes, r.indexChan(), "1 SOUR "+ptr)
	return true, nil
}

// AddFams ajoute "1 FAMS @X@" (individu conjoint d'une famille) si absent.
// Symétrique de AddFamc côté enfant : sert à réparer une famille où HUSB/WIFE
// pointe vers l'individu sans que celui-ci pointe en retour, ce qui fait
// disparaître la famille dans un lecteur qui part de l'individu. Renvoie true
// si ajoutée, false si déjà présente.
func (r *Record) AddFams(famXref string) bool {
	ptr := "@" + strings.Trim(famXref, "@") + "@"
	for _, ligne := range r.Lignes {
		if ligne == "1 FAMS "+ptr {
			return false
		}
	}
	r.Lignes = inserer(r.Lignes, r.indexChan(), "1 FAMS "+ptr)
	return true
}

// AddFamc ajoute "1 FAMC @X@" (individu enfant d'une famille) si absent. Symétrique
// de AddFams côté conjoint — répare une FAM qui porte l'individu en CHIL sans que
// celui-ci pointe en retour. Renvoie true si ajoutée, false si déjà présente.
func (r *Record) AddFamc(famXref string) bool {
	ptr := "@" + strings.Trim(famXref, "@") + "@"
	for _, ligne := range r.Lignes {
		if ligne == "1 FAMC "+ptr {
			return false
		}
	}
	r.Lignes = inserer(r.Lignes, r.indexChan(), "1 FAMC "+ptr)
	return true
}

// SupprimerOccurrenceEnTrop retire une occurrence surnuméraire d'une ligne EXACTE
// (ex. "1 FAMS @F0009@") quand elle apparaît plus d'une fois dans l'enregistrement —
// garde la première, retire la dernière. Ne fait rien (renvoie false) s'il n'y en a
// qu'une ou zéro occurrence. Utilisé par `fix` pour corriger D3/D4 (pointeur répété) :
// aucune information n'est perdue, le lien légitime subsiste une fois.
func (r *Record) SupprimerOccurrenceEnTrop(ligneExacte string) bool {
	dernier, compte := -1, 0
	for i, l := range r.Lignes {
		if l == ligneExacte {
			compte++
			dernier = i
		}
	}
	if compte < 2 {
		return false
	}
	r.Lignes = append(r.Lignes[:dernier], r.Lignes[dernier+1:]...)
	return true
}

// ReplierLigne remplace une ligne trop longue par son repli CONC (voir Enligne).
// Ne fait rien (renvoie false) si la ligne n'est pas trouvée telle quelle, ou si
// elle est malformée (hors périmètre : c'est alors S1, pas S3, qui la signale).
// Utilisé par `fix` pour corriger S3.
func (r *Record) ReplierLigne(ligneOriginale string) bool {
	for i, l := range r.Lignes {
		if l != ligneOriginale {
			continue
		}
		d, ok := Decoupe(l)
		if !ok {
			return false
		}
		repli := Enligne(d.Niveau, d.Tag, d.Valeur)
		out := make([]string, 0, len(r.Lignes)-1+len(repli))
		out = append(out, r.Lignes[:i]...)
		out = append(out, repli...)
		out = append(out, r.Lignes[i+1:]...)
		r.Lignes = out
		return true
	}
	return false
}

// AjouterLigne insère une ligne de niveau 1 juste avant le bloc "1 CHAN" (ou en fin
// d'enregistrement s'il n'y en a pas), sans vérifier si elle existe déjà —
// contrairement à AddFams/AddFamc/AddCitation, faites pour un pointeur qui ne doit
// apparaître qu'une fois. Sert à des contenus qui peuvent légitimement se répéter
// (ex. plusieurs "1 CHIL" sur une même FAM).
func (r *Record) AjouterLigne(ligne string) {
	r.Lignes = inserer(r.Lignes, r.indexChan(), ligne)
}

// AjouterLignes insère plusieurs lignes, dans l'ordre donné, juste avant "1 CHAN"
// (ou en fin d'enregistrement) — répète AjouterLigne ligne par ligne, ce qui bâtit
// correctement un bloc hiérarchique (ex. "1 BIRT" puis "2 DATE ...") : chaque
// insertion se fait juste avant CHAN, donc juste après la ligne insérée avant elle.
//
// Sert à ajouter un fait qu'une fiche n'avait pas du tout (BIRT, OCCU, NOTE...),
// ce qu'aucune autre primitive ne couvre : SetEventDate exige que l'événement
// existe déjà, AddCitation/AddFams/AddFamc sont pour des pointeurs qui ne doivent
// apparaître qu'une fois.
func (r *Record) AjouterLignes(lignes []string) {
	for _, l := range lignes {
		r.AjouterLigne(l)
	}
}

// RemplacerLigne remplace la première occurrence d'une ligne EXACTE par une autre.
// Renvoie false si non trouvée. Primitive générique pour `apply` (opération
// "set_line") : les correctifs déclaratifs n'ont pas tous un helper dédié.
func (r *Record) RemplacerLigne(ancienne, nouvelle string) bool {
	for i, l := range r.Lignes {
		if l == ancienne {
			r.Lignes[i] = nouvelle
			return true
		}
	}
	return false
}

// SupprimerLigne retire la première occurrence d'une ligne EXACTE. Renvoie false si
// non trouvée. Primitive générique pour `apply` (opération "remove_line").
func (r *Record) SupprimerLigne(ligne string) bool {
	for i, l := range r.Lignes {
		if l == ligne {
			r.Lignes = append(r.Lignes[:i], r.Lignes[i+1:]...)
			return true
		}
	}
	return false
}

func (r *Record) indexChan() int {
	for j, ligne := range r.Lignes {
		if ligne == "1 CHAN" {
			return j
		}
	}
	return len(r.Lignes)
}

// TouchChan met le bloc "1 CHAN" à la date du jour (ou à date/heure si fournies,
// format GEDCOM : "5 JUN 2026" / "11:06:53").
func (r *Record) TouchChan(date, heure string) {
	maintenant := time.Now()
	if date == "" {
		date = strings.ToUpper(maintenant.Format("2 Jan 2006"))
	}
	if heure == "" {
		heure = maintenant.Format("15:04:05")
	}
	j := r.indexChan()
	if j >= len(r.Lignes) {
		r.Lignes = append(r.Lignes, "1 CHAN", "2 DATE "+date, "3 TIME "+heure)
		return
	}
	for k := j + 1; k < len(r.Lignes); k++ {
		l, ok := Decoupe(r.Lignes[k])
		if !ok || l.Niveau < 2 {
			break
		}
		switch l.Tag {
		case "DATE":
			r.Lignes[k] = "2 DATE " + date
		case "TIME":
			r.Lignes[k] = "3 TIME " + heure
		}
	}
}

// Package add ajoute un individu à un GEDCOM en câblant tous ses liens de parenté de
// façon non équivoque : chaque lien fourni (père, mère, conjoint) est écrit dans les
// deux sens (FAM <-> INDI), jamais un seul. Ce paquet ne connaît rien des règles de
// validation — c'est à l'appelant (cmd_add.go) de rejouer le registre avant/après et
// de refuser d'écrire si l'ajout introduit un signalement nouveau.
package add

import (
	"fmt"
	"strings"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

// Requete décrit l'individu à ajouter. Nom est au format GEDCOM complet
// ("Jean /Dupret/") : prénom(s) avant la première barre oblique, patronyme entre les
// barres. Tous les autres champs sont optionnels ("" = inconnu/absent).
type Requete struct {
	Nom       string
	Sexe      string // "M", "F", ou "" si inconnu
	Naissance string // date GEDCOM, ex. "12 MAR 1805"
	Deces     string
	Pere      string // xref, "" si aucun
	Mere      string
	Conjoint  string
	Note      string // justification, écrite en "1 NOTE"

	IgnorerHomonymes bool // passe outre ErrHomonymes plutôt que de refuser
}

// Homonyme est un individu déjà présent qui pourrait être la même personne.
type Homonyme struct {
	Xref      string `json:"xref"`
	Nom       string `json:"nom"`
	Naissance string `json:"naissance,omitempty"`
}

// ErrHomonymes bloque l'ajout par défaut : Requete.IgnorerHomonymes=true (option
// --force côté CLI) passe outre en connaissance de cause.
type ErrHomonymes struct{ Homonymes []Homonyme }

func (e *ErrHomonymes) Error() string {
	noms := make([]string, len(e.Homonymes))
	for i, h := range e.Homonymes {
		noms[i] = fmt.Sprintf("%s (%s, né(e) %s)", h.Xref, h.Nom, blancSiVide(h.Naissance))
	}
	return "homonyme(s) potentiel(s) déjà présent(s) : " + strings.Join(noms, ", ") +
		" — relancer avec --force pour ajouter quand même"
}

func blancSiVide(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

// Resultat décrit ce qu'Ajouter a fait : Xref du nouvel individu, et Diff (les
// lignes ajoutées, dans l'ordre) pour affichage en simulation comme après écriture.
type Resultat struct {
	Xref      string     `json:"xref"`
	Diff      []string   `json:"diff"`
	Homonymes []Homonyme `json:"homonymes,omitempty"`
}

const fenetreHomonymesAnnees = 3

// ChercherHomonymes cherche des individus existants au patronyme et prénom
// équivalents (comparaison insensible à la casse et aux accents, voir
// gedcom.Normaliser), dans une fenêtre de quelques années autour de la naissance si
// elle est connue.
func ChercherHomonymes(g *gedcom.Gedcom, req Requete) []Homonyme {
	patronyme := gedcom.Normaliser(gedcom.PatronymeDeNom(req.Nom))
	if patronyme == "" {
		return nil
	}
	prenom := gedcom.Normaliser(gedcom.PrenomDeNom(req.Nom))
	naissance, okNaissance := gedcom.Annee(req.Naissance)

	var out []Homonyme
	for _, ind := range g.Individus() {
		if gedcom.Normaliser(ind.Patronyme()) != patronyme {
			continue
		}
		if prenom != "" && gedcom.Normaliser(gedcom.PrenomDeNom(ind.Valeur("NAME"))) != prenom {
			continue
		}
		if okNaissance {
			if an, ok := gedcom.Annee(ind.Date("BIRT")); ok && abs(an-naissance) > fenetreHomonymesAnnees {
				continue
			}
		}
		out = append(out, Homonyme{Xref: ind.Xref, Nom: ind.Nom(), Naissance: ind.Date("BIRT")})
	}
	return out
}

// Ajouter crée l'individu et câble ses liens de parenté, dans les deux sens :
// FAM.CHIL + INDI.FAMC pour les parents, FAM.HUSB/WIFE + INDI.FAMS pour le conjoint.
// N'écrit rien sur disque. Refuse (ErrHomonymes) si un homonyme potentiel existe et
// que Requete.IgnorerHomonymes n'est pas positionné.
func Ajouter(g *gedcom.Gedcom, req Requete) (*Resultat, error) {
	if strings.TrimSpace(req.Nom) == "" {
		return nil, fmt.Errorf("le nom est obligatoire")
	}
	if req.Pere != "" && !g.Contains(req.Pere) {
		return nil, fmt.Errorf("père inconnu : @%s@ n'existe pas", req.Pere)
	}
	if req.Mere != "" && !g.Contains(req.Mere) {
		return nil, fmt.Errorf("mère inconnue : @%s@ n'existe pas", req.Mere)
	}
	if req.Conjoint != "" && !g.Contains(req.Conjoint) {
		return nil, fmt.Errorf("conjoint inconnu : @%s@ n'existe pas", req.Conjoint)
	}

	homonymes := ChercherHomonymes(g, req)
	if len(homonymes) > 0 && !req.IgnorerHomonymes {
		return nil, &ErrHomonymes{Homonymes: homonymes}
	}

	xref := g.ProchainXref("I")
	var lignes []string
	lignes = append(lignes, "1 NAME "+req.Nom)
	if req.Sexe != "" {
		lignes = append(lignes, "1 SEX "+strings.ToUpper(req.Sexe))
	}
	if req.Naissance != "" {
		lignes = append(lignes, "1 BIRT", "2 DATE "+req.Naissance)
	}
	if req.Deces != "" {
		lignes = append(lignes, "1 DEAT", "2 DATE "+req.Deces)
	}
	if req.Note != "" {
		lignes = append(lignes, gedcom.EnligneNote(1, req.Note)...)
	}

	diff := []string{fmt.Sprintf("+ 0 @%s@ INDI", xref)}
	for _, l := range lignes {
		diff = append(diff, "+ "+l)
	}

	indi, err := g.AddIndividual(xref, lignes, "")
	if err != nil {
		return nil, err
	}

	if req.Pere != "" || req.Mere != "" {
		fam, creee, err := trouverOuCreerFamilleParent(g, req.Pere, req.Mere)
		if err != nil {
			return nil, err
		}
		diff = append(diff, creee...)
		// Réciprocité côté père/mère : sans ce FAMS, la famille "disparaît" pour un
		// lecteur qui part du parent — c'est le bug I0614/F0259 documenté dans
		// controle_liens.py, que cet ajout doit justement ne jamais reproduire.
		for _, parent := range []string{req.Pere, req.Mere} {
			if parent == "" {
				continue
			}
			if parentRec, ok := g.Get(parent); ok && parentRec.AddFams(fam.Xref) {
				diff = append(diff, fmt.Sprintf("+ (%s) 1 FAMS @%s@", parent, fam.Xref))
			}
		}
		fam.AjouterLigne("1 CHIL @" + xref + "@")
		diff = append(diff, fmt.Sprintf("+ (%s) 1 CHIL @%s@", fam.Xref, xref))
		indi.AddFamc(fam.Xref)
		diff = append(diff, fmt.Sprintf("+ (%s) 1 FAMC @%s@", xref, fam.Xref))
	}

	if req.Conjoint != "" {
		fam, creee, err := trouverOuCreerFamilleConjoint(g, xref, req.Conjoint, req.Sexe)
		if err != nil {
			return nil, err
		}
		diff = append(diff, creee...)
		if indi.AddFams(fam.Xref) {
			diff = append(diff, fmt.Sprintf("+ (%s) 1 FAMS @%s@", xref, fam.Xref))
		}
		if conjRec, ok := g.Get(req.Conjoint); ok && conjRec.AddFams(fam.Xref) {
			diff = append(diff, fmt.Sprintf("+ (%s) 1 FAMS @%s@", req.Conjoint, fam.Xref))
		}
	}

	return &Resultat{Xref: xref, Diff: diff, Homonymes: homonymes}, nil
}

// trouverOuCreerFamilleParent cherche une FAM portant exactement (pere, mere) en
// (HUSB, WIFE) — y compris quand l'un des deux est inconnu ("") — et n'en crée une
// que si aucune ne correspond. `creee` est le diff d'affichage, vide si trouvée.
func trouverOuCreerFamilleParent(g *gedcom.Gedcom, pere, mere string) (fam *gedcom.Record, creee []string, err error) {
	for _, f := range g.Familles() {
		if f.Valeur("HUSB") == pere && f.Valeur("WIFE") == mere {
			return f, nil, nil
		}
	}
	xref := g.ProchainXref("F")
	fam, err = g.AddFamily(xref, pere, mere, nil, "")
	if err != nil {
		return nil, nil, err
	}
	creee = []string{fmt.Sprintf("+ 0 @%s@ FAM", xref)}
	if pere != "" {
		creee = append(creee, fmt.Sprintf("+ (%s) 1 HUSB @%s@", xref, pere))
	}
	if mere != "" {
		creee = append(creee, fmt.Sprintf("+ (%s) 1 WIFE @%s@", xref, mere))
	}
	return fam, creee, nil
}

// trouverOuCreerFamilleConjoint cherche une FAM unissant déjà (nouveau, conjoint) —
// dans n'importe quel ordre HUSB/WIFE — et n'en crée une que si aucune ne
// correspond. Le sexe du nouvel individu (ou, à défaut, celui du conjoint déjà
// connu) décide qui est HUSB et qui est WIFE.
func trouverOuCreerFamilleConjoint(g *gedcom.Gedcom, nouveau, conjoint, sexe string) (fam *gedcom.Record, creee []string, err error) {
	for _, f := range g.Familles() {
		h, w := f.Valeur("HUSB"), f.Valeur("WIFE")
		if (h == nouveau && w == conjoint) || (h == conjoint && w == nouveau) {
			return f, nil, nil
		}
	}
	husb, wife := nouveau, conjoint
	switch {
	case sexe == "F":
		husb, wife = conjoint, nouveau
	case sexe != "M":
		if r, ok := g.Get(conjoint); ok && r.Valeur("SEX") == "M" {
			husb, wife = conjoint, nouveau
		}
	}
	xref := g.ProchainXref("F")
	fam, err = g.AddFamily(xref, husb, wife, nil, "")
	if err != nil {
		return nil, nil, err
	}
	creee = []string{
		fmt.Sprintf("+ 0 @%s@ FAM", xref),
		fmt.Sprintf("+ (%s) 1 HUSB @%s@", xref, husb),
		fmt.Sprintf("+ (%s) 1 WIFE @%s@", xref, wife),
	}
	return fam, creee, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

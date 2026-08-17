// Package patch applique des correctifs déclaratifs — un fichier JSON qui décrit une
// modification, plutôt qu'un script Python à usage unique (le flux patch_*.py de
// l'ancien outillage). Les préconditions reproduisent les `assert` d'état initial de
// ces scripts : un correctif déjà appliqué refuse de se rejouer, exactement comme eux.
package patch

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

func firstOr(lignes []string, defaut string) string {
	if len(lignes) == 0 {
		return defaut
	}
	return lignes[0]
}

// Precondition vérifie un fait sur le GEDCOM avant toute modification. Deux formes :
// (Evenement, DateVaut) teste la DATE d'un événement ; (Tag, ValeurVaut) teste la
// valeur d'un champ de niveau 1 quelconque. Une seule des deux doit être renseignée.
type Precondition struct {
	Xref       string `json:"xref"`
	Evenement  string `json:"evenement,omitempty"`
	DateVaut   string `json:"date_vaut,omitempty"`
	Tag        string `json:"tag,omitempty"`
	ValeurVaut string `json:"valeur_vaut,omitempty"`
}

// Verifier renvoie une erreur si le fait attendu n'est pas constaté — le message
// suggère explicitement "déjà appliqué ?", le cas le plus fréquent en pratique
// (rejouer un correctif après qu'il a déjà pris effet).
func (p Precondition) Verifier(g *gedcom.Gedcom) error {
	rec, ok := g.Get(p.Xref)
	if !ok {
		return fmt.Errorf("précondition : @%s@ n'existe pas", p.Xref)
	}
	switch {
	case p.Evenement != "":
		if actuelle := rec.Date(p.Evenement); actuelle != p.DateVaut {
			return fmt.Errorf("précondition non remplie sur %s : %s.DATE = %q, attendu %q (déjà appliqué ?)",
				p.Xref, p.Evenement, actuelle, p.DateVaut)
		}
	case p.Tag != "":
		if actuelle := rec.Valeur(p.Tag); actuelle != p.ValeurVaut {
			return fmt.Errorf("précondition non remplie sur %s : %s = %q, attendu %q (déjà appliqué ?)",
				p.Xref, p.Tag, actuelle, p.ValeurVaut)
		}
	default:
		return fmt.Errorf("précondition invalide sur %s : ni evenement ni tag précisé", p.Xref)
	}
	return nil
}

// Operation modifie le GEDCOM. Op sélectionne l'action ; les autres champs utiles
// dépendent d'Op (voir Appliquer). Reprend exactement les primitives déjà exposées
// par le paquet gedcom — aucune opération sans équivalent direct dans son API.
type Operation struct {
	Op        string `json:"op"`
	Xref      string `json:"xref,omitempty"`
	Evenement string `json:"evenement,omitempty"`
	Valeur    string `json:"valeur,omitempty"`
	Source    string `json:"source,omitempty"`
	Fam       string `json:"fam,omitempty"`

	// add_source
	Titl  string `json:"titl,omitempty"`
	Auth  string `json:"auth,omitempty"`
	Publ  string `json:"publ,omitempty"`
	Note  string `json:"note,omitempty"`
	Apres string `json:"apres,omitempty"`

	// add_individual / add_family
	LignesNiveau1 []string `json:"lignes_niveau1,omitempty"`
	Husb          string   `json:"husb,omitempty"`
	Wife          string   `json:"wife,omitempty"`
	Chil          []string `json:"chil,omitempty"`

	// set_line / remove_line
	Ligne         string `json:"ligne,omitempty"`
	NouvelleLigne string `json:"nouvelle_ligne,omitempty"`

	// add_record : un enregistrement entier déjà mis en forme, lignes[0] = "0 @X@ TAG"
	// (utilisé par le plan de fusion produit par `merge --analyse`)
	Lignes []string `json:"lignes,omitempty"`
}

// Appliquer exécute l'opération sur g. Ne connaît que les primitives déjà exposées
// par le paquet gedcom (SetEventDate, AddCitation, AddFams...) : apply est une
// couche déclarative par-dessus l'API existante, pas une seconde implémentation.
func (o Operation) Appliquer(g *gedcom.Gedcom) error {
	if o.Op == "add_source" {
		_, err := g.AddSource(o.Xref, o.Titl, o.Auth, o.Publ, o.Note, o.Apres)
		return err
	}
	if o.Op == "add_individual" {
		_, err := g.AddIndividual(o.Xref, o.LignesNiveau1, o.Apres)
		return err
	}
	if o.Op == "add_family" {
		_, err := g.AddFamily(o.Xref, o.Husb, o.Wife, o.Chil, o.Apres)
		return err
	}
	if o.Op == "add_record" {
		if len(o.Lignes) == 0 || !strings.HasPrefix(o.Lignes[0], "0 ") {
			return fmt.Errorf("add_record : lignes[0] doit être \"0 @X@ TAG\", obtenu %q", firstOr(o.Lignes, ""))
		}
		_, err := g.InsererRecord(gedcom.NewRecord(append([]string{}, o.Lignes...)), o.Apres)
		return err
	}

	rec, ok := g.Get(o.Xref)
	if !ok {
		return fmt.Errorf("%q sur @%s@ : n'existe pas", o.Op, o.Xref)
	}
	switch o.Op {
	case "set_event_date":
		_, err := rec.SetEventDate(o.Evenement, o.Valeur)
		return err
	case "add_citation":
		_, err := rec.AddCitation(o.Source, o.Evenement)
		return err
	case "add_fams":
		rec.AddFams(o.Fam)
		return nil
	case "add_famc":
		rec.AddFamc(o.Fam)
		return nil
	case "set_line":
		if !rec.RemplacerLigne(o.Ligne, o.NouvelleLigne) {
			return fmt.Errorf("%s : ligne %q introuvable", o.Xref, o.Ligne)
		}
		return nil
	case "remove_line":
		if !rec.SupprimerLigne(o.Ligne) {
			return fmt.Errorf("%s : ligne %q introuvable", o.Xref, o.Ligne)
		}
		return nil
	case "touch_chan":
		rec.TouchChan("", "") // toujours la date du jour, comme g.touch_chan() dans les patchs Python
		return nil
	default:
		return fmt.Errorf("opération inconnue : %q", o.Op)
	}
}

// Correctif est un fichier de correctif déclaratif complet.
type Correctif struct {
	Cible         string         `json:"cible"`
	Justification string         `json:"justification"`
	Preconditions []Precondition `json:"preconditions,omitempty"`
	Operations    []Operation    `json:"operations"`
}

// Charger lit et décode un fichier de correctif JSON.
func Charger(chemin string) (*Correctif, error) {
	octets, err := os.ReadFile(chemin)
	if err != nil {
		return nil, err
	}
	var c Correctif
	if err := json.Unmarshal(octets, &c); err != nil {
		return nil, fmt.Errorf("%s : %w", chemin, err)
	}
	if len(c.Operations) == 0 {
		return nil, fmt.Errorf("%s : aucune opération", chemin)
	}
	return &c, nil
}

// Appliquer vérifie d'abord TOUTES les préconditions (rien n'est modifié tant
// qu'une seule n'est pas remplie), puis exécute les opérations dans l'ordre. Ne
// touche jamais le disque : c'est à l'appelant (cmd_apply.go) de charger le GEDCOM,
// appeler Appliquer, et ne sauvegarder qu'en cas de succès complet — une opération
// en échec laisse donc, au pire, un état intermédiaire en mémoire, jamais écrit.
func (c *Correctif) Appliquer(g *gedcom.Gedcom) error {
	for _, p := range c.Preconditions {
		if err := p.Verifier(g); err != nil {
			return err
		}
	}
	for _, op := range c.Operations {
		if err := op.Appliquer(g); err != nil {
			return fmt.Errorf("opération %q : %w", op.Op, err)
		}
	}
	return nil
}

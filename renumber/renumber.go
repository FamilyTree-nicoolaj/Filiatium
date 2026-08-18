// Package renumber calcule une renumérotation complète des xref INDI/FAM d'un
// GEDCOM (jamais SOUR/NOTE/OBJE/SUBM/REPO, qui gardent les leurs), selon l'une de
// trois stratégies (Calculer, CalculerDecalage, CalculerPrefixe), et sait rejouer
// la correspondance obtenue sur du texte libre (AppliquerNotes) pour mettre à jour
// des notes de recherche externes au GEDCOM. Ne modifie jamais le *gedcom.Gedcom
// qu'on lui passe : chaque fonction renvoie une table ancien xref -> nouveau xref,
// à appliquer séparément via gedcom.Gedcom.Renumeroter.
package renumber

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

// Table est la correspondance produite par une renumérotation — sérialisée pour
// --table (mode A de `renumber`) et relue par --depuis-table (mode B).
type Table struct {
	Cible          string            `json:"cible"`
	Strategie      string            `json:"strategie"`      // "source" | "decalage" | "prefixe"
	Parametre      string            `json:"parametre"`      // xref racine, "5000", ou "Z" selon Strategie
	Correspondance map[string]string `json:"correspondance"` // ancien xref -> nouveau, INDI+FAM uniquement
}

// -------------------------------------------------------- stratégie --source (BFS)

// numeroteur porte l'état d'un parcours en largeur : compteurs et largeurs déjà
// figés à partir du total d'individus/familles, table en cours de remplissage.
type numeroteur struct {
	g                    *gedcom.Gedcom
	table                map[string]string
	largeurI, largeurF   int
	prochainI, prochainF int
}

// largeur renvoie le nombre de chiffres nécessaire pour n, avec un plancher à 4 —
// même esprit que gedcom.ProchainXref, sans son rescan incrémental puisque le
// total est connu dès le départ.
func largeur(n int) int {
	l := len(strconv.Itoa(n))
	if l < 4 {
		return 4
	}
	return l
}

func (n *numeroteur) indi(xref string) string {
	if nv, ok := n.table[xref]; ok {
		return nv
	}
	nv := fmt.Sprintf("I%0*d", n.largeurI, n.prochainI)
	n.prochainI++
	n.table[xref] = nv
	return nv
}

func (n *numeroteur) fam(xref string) string {
	if nv, ok := n.table[xref]; ok {
		return nv
	}
	nv := fmt.Sprintf("F%0*d", n.largeurF, n.prochainF)
	n.prochainF++
	n.table[xref] = nv
	return nv
}

// enfiler ajoute xref à file s'il n'est pas vide — un pointeur pendant (vérifié par
// l'appelant via g.Get avant d'enfiler la famille elle-même) ne doit jamais
// consommer un créneau de numérotation.
func enfiler(file []string, xref string) []string {
	if xref != "" {
		file = append(file, xref)
	}
	return file
}

// bfs numérote depart (s'il ne l'est pas déjà) et tout ce qui lui est atteignable
// par FAMC/FAMS — conjoints, enfants, parents (toutes pédigrées, pas seulement la
// filiation biologique, contrairement à gedcom.Sosa). No-op si depart est déjà
// numéroté : un appel = une composante connexe. Les voisins viennent toujours de
// Valeurs(...) (ordre des lignes du fichier), jamais d'un parcours de map : bfs
// est déterministe.
func (n *numeroteur) bfs(depart string) {
	if _, vu := n.table[depart]; vu {
		return
	}
	file := []string{depart}
	for len(file) > 0 {
		x := file[0]
		file = file[1:]
		if _, vu := n.table[x]; vu {
			continue
		}
		rec, ok := n.g.Get(x)
		if !ok || rec.Tag != "INDI" {
			continue // pointeur pendant : jamais numéroté, jamais réécrit
		}
		n.indi(x)

		for _, famc := range rec.Valeurs("FAMC") {
			fam, ok := n.g.Get(famc)
			if !ok {
				continue
			}
			n.fam(famc)
			file = enfiler(file, fam.Valeur("HUSB"))
			file = enfiler(file, fam.Valeur("WIFE"))
		}
		for _, fams := range rec.Valeurs("FAMS") {
			fam, ok := n.g.Get(fams)
			if !ok {
				continue
			}
			n.fam(fams)
			file = enfiler(file, fam.Valeur("HUSB"))
			file = enfiler(file, fam.Valeur("WIFE"))
			for _, chil := range fam.Valeurs("CHIL") {
				file = enfiler(file, chil)
			}
		}
	}
}

// Calculer calcule la table de renumérotation complète (INDI+FAM) de g, en partant
// de source (un INDI de g) par parcours en largeur, puis balaie g.Individus()/
// g.Familles() en ordre fichier pour couvrir toute composante déconnectée et toute
// FAM orpheline restante (rattachée à aucun INDI). Ne modifie jamais g ; fonction
// pure de (source, contenu de g) : même entrée, même table, toujours.
func Calculer(g *gedcom.Gedcom, source string) (map[string]string, error) {
	source = strings.Trim(source, "@")
	rec, ok := g.Get(source)
	if !ok || rec.Tag != "INDI" {
		return nil, fmt.Errorf("@%s@ n'est pas un individu de ce fichier", source)
	}

	n := &numeroteur{
		g: g, table: map[string]string{},
		largeurI: largeur(len(g.Individus())), largeurF: largeur(len(g.Familles())),
		prochainI: 1, prochainF: 1,
	}
	n.bfs(source)
	for _, ind := range g.Individus() {
		n.bfs(ind.Xref) // composantes déconnectées, en ordre fichier
	}
	for _, fam := range g.Familles() {
		if _, vu := n.table[fam.Xref]; !vu {
			n.fam(fam.Xref) // FAM orpheline, rattachée à aucun INDI
		}
	}

	if err := validerTable(g, n.table); err != nil {
		return nil, err
	}
	return n.table, nil
}

// -------------------------------------------- stratégies --decalage et --prefixe

var xrefLettreNumRe = regexp.MustCompile(`^([A-Za-z]+)(\d+)$`)

func enregistrementsARenumeroter(g *gedcom.Gedcom) []*gedcom.Record {
	out := append([]*gedcom.Record{}, g.Individus()...)
	return append(out, g.Familles()...)
}

// CalculerDecalage décale de n le numéro de chaque xref INDI/FAM existant, en
// gardant sa lettre de tag et au moins sa largeur d'origine (%0*d — un décalage
// qui dépasse la largeur d'origine s'étend naturellement, jamais tronqué). Erreur
// si un xref ne suit pas le motif lettre+chiffres, ou si le résultat décalé
// devient négatif.
func CalculerDecalage(g *gedcom.Gedcom, n int) (map[string]string, error) {
	table := map[string]string{}
	for _, rec := range enregistrementsARenumeroter(g) {
		m := xrefLettreNumRe.FindStringSubmatch(rec.Xref)
		if m == nil {
			return nil, fmt.Errorf("%s : xref %q ne suit pas le motif lettre+chiffres, --decalage inapplicable", rec.Tag, rec.Xref)
		}
		lettre, chiffres := m[1], m[2]
		v, _ := strconv.Atoi(chiffres) // motif garantit un entier
		nv := v + n
		if nv < 0 {
			return nil, fmt.Errorf("%s : décalage de %d ferait passer %s sous zéro", rec.Tag, n, rec.Xref)
		}
		table[rec.Xref] = fmt.Sprintf("%s%0*d", lettre, len(chiffres), nv)
	}
	if err := validerTable(g, table); err != nil {
		return nil, err
	}
	return table, nil
}

// CalculerPrefixe ajoute lettre devant chaque xref INDI/FAM existant tel quel (ex.
// "Z"+"I0001" -> "ZI0001") — concaténation pure, toujours injective tant que
// lettre est non vide.
func CalculerPrefixe(g *gedcom.Gedcom, lettre string) (map[string]string, error) {
	if lettre == "" {
		return nil, fmt.Errorf("--prefixe requiert une lettre non vide")
	}
	table := map[string]string{}
	for _, rec := range enregistrementsARenumeroter(g) {
		table[rec.Xref] = lettre + rec.Xref
	}
	if err := validerTable(g, table); err != nil {
		return nil, err
	}
	return table, nil
}

// validerTable vérifie que table est une bijection (deux xref d'origine différents
// ne pointent jamais vers le même nouveau xref) et qu'aucun nouveau xref n'entre en
// collision avec un enregistrement non renuméroté (SOUR/OBJE/NOTE/SUBM/REPO, qui
// gardent leur xref d'origine). Peu probable sur un fichier I/F propre, mais un
// décalage ou un préfixe mal choisi doit échouer proprement plutôt que produire un
// fichier corrompu — appelée une fois, à la fin de chaque stratégie.
func validerTable(g *gedcom.Gedcom, table map[string]string) error {
	vus := make(map[string]string, len(table))
	for ancien, nouveau := range table {
		if autre, deja := vus[nouveau]; deja {
			return fmt.Errorf("collision : %s et %s se renuméroteraient tous deux en %s", autre, ancien, nouveau)
		}
		vus[nouveau] = ancien
	}
	for _, r := range g.Records {
		if _, renumerote := table[r.Xref]; renumerote {
			continue
		}
		if ancien, prisParTable := vus[r.Xref]; prisParTable {
			return fmt.Errorf("collision : %s (non renuméroté) coïnciderait avec le nouveau xref de %s", r.Xref, ancien)
		}
	}
	return nil
}

// ------------------------------------------------------------------------ notes

// AppliquerNotes remplace, en mot entier, chaque clé de correspondance par sa
// valeur dans contenu — jamais un motif générique (type "[IF]\d+"), seulement les
// xref réellement issus de cette renumérotation précise : aucun faux positif
// possible. \b tombe aussi bien entre lettres qu'entre un "@" et un xref, donc un
// xref cité "@F0271@" (ligne GEDCOM brute reproduite dans une note) est reconnu
// sans traitement particulier. n compte les remplacements effectués.
func AppliquerNotes(contenu string, correspondance map[string]string) (nouveau string, n int) {
	if len(correspondance) == 0 {
		return contenu, 0
	}
	motifs := make([]string, 0, len(correspondance))
	for x := range correspondance {
		motifs = append(motifs, regexp.QuoteMeta(x))
	}
	sort.Strings(motifs) // ordre déterministe du motif compilé
	re := regexp.MustCompile(`\b(` + strings.Join(motifs, "|") + `)\b`)
	nouveau = re.ReplaceAllStringFunc(contenu, func(m string) string {
		n++
		return correspondance[m]
	})
	return nouveau, n
}

// Package merge analyse si deux GEDCOM peuvent se fusionner, et à quel prix. Il
// n'écrit jamais de GEDCOM lui-même : Analyser produit un rapport, et Plan produit un
// correctif déclaratif (patch.Correctif) que l'utilisateur relit puis exécute via
// `apply` — aucun second mécanisme d'écriture n'existe à côté de celui-là.
package merge

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/patch"
	"github.com/FamilyTree-nicoolaj/filiatium/rules"
)

// PrefixeParDefaut préfixe les xref de l'apport lors de la renumérotation proposée
// ("I0001" -> "BI0001") — changez-le si "B" collide déjà.
const PrefixeParDefaut = "B"

// Classe qualifie la confiance d'un appariement entre deux individus.
type Classe string

const (
	Certaine  Classe = "certaine"
	Probable  Classe = "probable"
	AExaminer Classe = "à examiner"
)

// Appariement propose que XrefBase (dans `base`) et XrefApport (dans `apport`)
// désignent la même personne. Criteres explique ce qui a compté pour le score ;
// Conflits liste les faits qui s'y opposent (jamais tranchés automatiquement).
type Appariement struct {
	XrefBase   string   `json:"xref_base"`
	XrefApport string   `json:"xref_apport"`
	NomBase    string   `json:"nom_base"`
	NomApport  string   `json:"nom_apport"`
	Score      int      `json:"score"`
	Classe     Classe   `json:"classe"`
	Criteres   []string `json:"criteres,omitempty"`
	Conflits   []string `json:"conflits,omitempty"`
}

// Collisions compte les xref que `base` et `apport` définissent tous les deux.
type Collisions struct {
	Individus int `json:"individus"`
	Familles  int `json:"familles"`
	Sources   int `json:"sources"`
}

func (c Collisions) Total() int { return c.Individus + c.Familles + c.Sources }

// Analyse est le résultat complet de Analyser.
type Analyse struct {
	Collisions         Collisions    `json:"collisions"`
	PrefixeSuggere     string        `json:"prefixe_suggere"`
	Appariements       []Appariement `json:"appariements"`                   // triés par score décroissant
	NouveauxApresMerge []string      `json:"nouveaux_apres_merge,omitempty"` // signalements S*/L*/D*/R* introduits par la seule concaténation mécanique
	Verdict            string        `json:"verdict"`
}

// Analyser compare base et apport et produit le rapport complet. N'écrit rien.
func Analyser(base, apport *gedcom.Gedcom) *Analyse {
	prefixe := PrefixeParDefaut
	collisions := detecterCollisions(base, apport)
	appariements := apparier(base, apport)

	apportR := renumeroter(apport, prefixe)
	seuils := config.Defauts()
	avant := unionMessages(toutesRegles(base, seuils), toutesRegles(apportR, seuils))
	fusion := concatener(base, apportR)
	apres := toutesRegles(fusion, seuils)
	nouveaux := messagesNonPresents(apres, avant)

	a := &Analyse{
		Collisions:         collisions,
		PrefixeSuggere:     prefixe,
		Appariements:       appariements,
		NouveauxApresMerge: nouveaux,
	}
	a.Verdict = etablirVerdict(a)
	return a
}

func etablirVerdict(a *Analyse) string {
	nCertaines, nProbables, nConflits := 0, 0, 0
	for _, app := range a.Appariements {
		switch app.Classe {
		case Certaine:
			nCertaines++
		case Probable:
			nProbables++
		}
		if len(app.Conflits) > 0 {
			nConflits++
		}
	}
	switch {
	case len(a.NouveauxApresMerge) > 0:
		return fmt.Sprintf("à arbitrer : la fusion mécanique introduit %d signalement(s) nouveau(x)", len(a.NouveauxApresMerge))
	case nConflits > 0:
		return fmt.Sprintf("à arbitrer : %d appariement(s) avec conflit de faits", nConflits)
	case a.Collisions.Total() > 0:
		return fmt.Sprintf("fusionnable après renumérotation (préfixe %q) — %d certaine(s), %d probable(s)",
			a.PrefixeSuggere, nCertaines, nProbables)
	default:
		return fmt.Sprintf("fusionnable tel quel — %d certaine(s), %d probable(s)", nCertaines, nProbables)
	}
}

func detecterCollisions(base, apport *gedcom.Gedcom) Collisions {
	baseX := base.ParXref()
	var c Collisions
	for _, r := range apport.Records {
		if _, existe := baseX[r.Xref]; !existe {
			continue
		}
		switch r.Tag {
		case "INDI":
			c.Individus++
		case "FAM":
			c.Familles++
		case "SOUR":
			c.Sources++
		}
	}
	return c
}

// ---------------------------------------------------------------- appariement

const seuilAffichage = 20
const bonusParente = 20

func apparier(base, apport *gedcom.Gedcom) []Appariement {
	index := map[string][]*gedcom.Record{}
	for _, ind := range base.Individus() {
		p := gedcom.Normaliser(ind.Patronyme())
		if p != "" {
			index[p] = append(index[p], ind)
		}
	}

	var brut []Appariement
	for _, ind2 := range apport.Individus() {
		p := gedcom.Normaliser(ind2.Patronyme())
		for _, ind1 := range index[p] {
			sc, crit, confl := scorer(ind1, ind2)
			if sc <= 0 {
				continue
			}
			brut = append(brut, Appariement{
				XrefBase: ind1.Xref, XrefApport: ind2.Xref,
				NomBase: ind1.Nom(), NomApport: ind2.Nom(),
				Score: sc, Criteres: crit, Conflits: confl,
			})
		}
	}

	// meilleur appariement provisoire par xref d'apport, pour la propagation par
	// la parenté (deuxième passe ci-dessous)
	meilleur := map[string]Appariement{}
	for _, a := range brut {
		if cur, ok := meilleur[a.XrefApport]; !ok || a.Score > cur.Score {
			meilleur[a.XrefApport] = a
		}
	}

	var out []Appariement
	for _, a := range brut {
		ind1, _ := base.Get(a.XrefBase)
		ind2, _ := apport.Get(a.XrefApport)
		bonus, crit := scorerParente(base, apport, ind1, ind2, meilleur)
		a.Score += bonus
		a.Criteres = append(a.Criteres, crit...)
		a.Classe = classer(a.Score, a.Conflits)
		if a.Score >= seuilAffichage || len(a.Conflits) > 0 {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].XrefApport < out[j].XrefApport
	})
	return out
}

func classer(score int, conflits []string) Classe {
	if len(conflits) > 0 {
		return AExaminer
	}
	switch {
	case score >= 70:
		return Certaine
	case score >= 40:
		return Probable
	default:
		return AExaminer
	}
}

func scorer(ind1, ind2 *gedcom.Record) (score int, criteres, conflits []string) {
	patro1, patro2 := gedcom.Normaliser(ind1.Patronyme()), gedcom.Normaliser(ind2.Patronyme())
	if patro1 == "" || patro1 != patro2 {
		return 0, nil, nil
	}
	score += 20
	criteres = append(criteres, "patronyme identique")

	prenom1 := gedcom.Normaliser(gedcom.PrenomDeNom(ind1.Valeur("NAME")))
	prenom2 := gedcom.Normaliser(gedcom.PrenomDeNom(ind2.Valeur("NAME")))
	switch {
	case prenom1 != "" && prenom1 == prenom2:
		score += 40
		criteres = append(criteres, "prénom identique")
	case prenom1 != "" && prenom2 != "":
		conflits = append(conflits, fmt.Sprintf("prénom différent : %q vs %q", prenom1, prenom2))
	}

	a1, ok1 := gedcom.Annee(ind1.Date("BIRT"))
	a2, ok2 := gedcom.Annee(ind2.Date("BIRT"))
	switch {
	case ok1 && ok2 && a1 == a2:
		score += 25
		criteres = append(criteres, fmt.Sprintf("naissance identique (%d)", a1))
	case ok1 && ok2 && abs(a1-a2) <= 2:
		score += 10
		criteres = append(criteres, fmt.Sprintf("naissance proche (%d vs %d)", a1, a2))
	case ok1 && ok2:
		conflits = append(conflits, fmt.Sprintf("naissance différente : %d vs %d", a1, a2))
	default:
		score += 5 // l'une des deux inconnue : ni pour ni contre
	}

	if lieu1, lieu2 := lieuNaissance(ind1), lieuNaissance(ind2); lieu1 != "" && lieu1 == lieu2 {
		score += 10
		criteres = append(criteres, "lieu de naissance identique")
	}

	if s1, s2 := ind1.Valeur("SEX"), ind2.Valeur("SEX"); s1 != "" && s2 != "" && s1 != s2 {
		conflits = append(conflits, fmt.Sprintf("sexe différent : %s vs %s", s1, s2))
	}

	return score, criteres, conflits
}

func lieuNaissance(ind *gedcom.Record) string {
	ev := ind.Evenement("BIRT")
	if ev == nil {
		return ""
	}
	return gedcom.Normaliser(ev.Lieu())
}

// scorerParente ajoute un bonus quand le père et/ou la mère (filiation biologique)
// de ind1/ind2 sont eux-mêmes le meilleur appariement provisoire l'un de l'autre —
// deux personnes dont les parents concordent sont plus vraisemblablement la même
// personne que ce que leurs seuls nom/date suggèrent.
func scorerParente(base, apport *gedcom.Gedcom, ind1, ind2 *gedcom.Record, meilleur map[string]Appariement) (int, []string) {
	if ind1 == nil || ind2 == nil {
		return 0, nil
	}
	p1, err1 := base.Parents(ind1.Xref, "birth")
	p2, err2 := apport.Parents(ind2.Xref, "birth")
	if err1 != nil || err2 != nil || len(p1) == 0 || len(p2) == 0 {
		return 0, nil
	}
	pere1, mere1 := p1[0][0], p1[0][1]
	pere2, mere2 := p2[0][0], p2[0][1]

	bonus := 0
	var crit []string
	if pere2 != "" && pere1 != "" {
		if m, ok := meilleur[pere2]; ok && m.XrefBase == pere1 {
			bonus += bonusParente
			crit = append(crit, "père déjà apparié")
		}
	}
	if mere2 != "" && mere1 != "" {
		if m, ok := meilleur[mere2]; ok && m.XrefBase == mere1 {
			bonus += bonusParente
			crit = append(crit, "mère déjà appariée")
		}
	}
	return bonus, crit
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// --------------------------------------------------- fusion hypothétique (contrôle)

var pointeurRe = regexp.MustCompile(`@([A-Za-z0-9_]+)@`)

// renumeroter renvoie une copie de g dont tous les xref sont préfixés (et tous les
// pointeurs internes réécrits en conséquence), pour ne jamais entrer en collision
// avec `base` lors d'une fusion. Concaténation directe, sans séparateur : la
// grammaire des xref (voir gedcom.Decoupe) n'admet que des caractères \w — un tiret
// romprait le round-trip de lecture, comme dans gedcom.py.
func renumeroter(g *gedcom.Gedcom, prefixe string) *gedcom.Gedcom {
	mapping := map[string]string{}
	for xref := range g.ParXref() {
		mapping[xref] = prefixe + xref
	}
	var records []*gedcom.Record
	for _, r := range g.Records {
		lignes := make([]string, len(r.Lignes))
		for i, l := range r.Lignes {
			lignes[i] = pointeurRe.ReplaceAllStringFunc(l, func(m string) string {
				x := m[1 : len(m)-1]
				if nv, ok := mapping[x]; ok {
					return "@" + nv + "@"
				}
				return m
			})
		}
		records = append(records, gedcom.NewRecord(lignes))
	}
	return &gedcom.Gedcom{Records: records}
}

// concatener assemble base et apportRenumerote en un seul Gedcom en mémoire : les
// enregistrements de l'apport (hors HEAD/TRLR) prennent place avant le TRLR de base.
// Uniquement pour l'analyse — jamais sauvegardé tel quel (voir Plan pour la version
// écrite via `apply`).
func concatener(base, apportRenumerote *gedcom.Gedcom) *gedcom.Gedcom {
	var records []*gedcom.Record
	var trlr *gedcom.Record
	for _, r := range base.Records {
		if r.Tag == "TRLR" {
			trlr = r
			continue
		}
		records = append(records, r)
	}
	for _, r := range apportRenumerote.Records {
		if r.Tag == "HEAD" || r.Tag == "TRLR" {
			continue
		}
		records = append(records, r)
	}
	if trlr != nil {
		records = append(records, trlr)
	}
	return &gedcom.Gedcom{Records: records}
}

func toutesRegles(g *gedcom.Gedcom, seuils config.Seuils) []rules.Finding {
	var out []rules.Finding
	for _, r := range rules.Registre {
		out = append(out, r.Verifie(g, seuils)...)
	}
	return out
}

func unionMessages(ensembles ...[]rules.Finding) map[string]bool {
	m := map[string]bool{}
	for _, findings := range ensembles {
		for _, f := range findings {
			m[f.Regle+"|"+f.Message] = true
		}
	}
	return m
}

func messagesNonPresents(findings []rules.Finding, deja map[string]bool) []string {
	var out []string
	vus := map[string]bool{}
	for _, f := range findings {
		cle := f.Regle + "|" + f.Message
		if deja[cle] || vus[cle] {
			continue
		}
		vus[cle] = true
		out = append(out, f.Regle+" : "+f.Message)
	}
	sort.Strings(out)
	return out
}

// -------------------------------------------------------------------------- plan

// Plan construit le correctif déclaratif qui réaliserait la fusion mécanique :
// insérer, dans `base`, chaque enregistrement de `apport` renuméroté avec le préfixe
// suggéré. N'identifie pas les doublons entre les deux arbres — c'est aux
// Appariements *certaine* de guider une relecture avant d'exécuter ce plan avec
// `apply --write` (fusionner deux fiches reste un jugement humain, jamais automatique).
func Plan(base, apport *gedcom.Gedcom, cheminBaseAffiche string, prefixe string) *patch.Correctif {
	if prefixe == "" {
		prefixe = PrefixeParDefaut
	}
	apportR := renumeroter(apport, prefixe)
	var ops []patch.Operation
	for _, r := range apportR.Records {
		if r.Tag == "HEAD" || r.Tag == "TRLR" {
			continue
		}
		ops = append(ops, patch.Operation{Op: "add_record", Lignes: append([]string{}, r.Lignes...)})
	}
	return &patch.Correctif{
		Cible: cheminBaseAffiche,
		Justification: fmt.Sprintf(
			"Plan de fusion généré par `merge --analyse` (préfixe %q) — insère %d enregistrement(s) "+
				"renumérotés depuis l'apport. Ne fusionne aucune fiche identifiée comme doublon : "+
				"relire le rapport d'appariement avant d'exécuter ce plan avec `apply --write`.",
			prefixe, len(ops)),
		Operations: ops,
	}
}

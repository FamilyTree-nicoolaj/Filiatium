// Package merge analyse si deux GEDCOM peuvent se fusionner, et à quel prix. Il
// n'écrit jamais de GEDCOM lui-même : Analyser produit un rapport, et Plan produit un
// correctif déclaratif (patch.Correctif) que l'utilisateur relit puis exécute via
// `apply` — aucun second mécanisme d'écriture n'existe à côté de celui-là.
//
// L'identité entre deux enregistrements ne se déduit jamais de leurs xref (qui
// peuvent coïncider par accident, comme deux exports d'une même base Gramps, ou au
// contraire diverger totalement) : uniquement de leur contenu (signature masquée,
// score nom/date/lieu/parenté) ou de leurs relations déjà établies (couple/enfants
// pour les familles).
package merge

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/FamilyTree-nicoolaj/filiatium/config"
	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
	"github.com/FamilyTree-nicoolaj/filiatium/patch"
	"github.com/FamilyTree-nicoolaj/filiatium/rules"
)

// Classe qualifie la confiance d'un appariement entre deux individus.
type Classe string

const (
	Certaine  Classe = "certaine"
	Probable  Classe = "probable"
	AExaminer Classe = "à examiner"
)

// Niveau borne ce que le plan de fusion incorpore réellement : chaque cran est un
// sur-ensemble du précédent (une fusion "probables" inclut aussi les "certaines").
// Le rapport, lui, liste toujours tous les appariements quel que soit le niveau —
// seul le plan est filtré.
type Niveau int

const (
	NiveauIdentiques Niveau = iota // uniquement le contenu octet-identique, aucun jugement
	NiveauCertaines                // + appariements certains (individus et familles qui en découlent)
	NiveauProbables                // + appariements probables (score 40-69)
	NiveauTout                     // + appariements "à examiner"
)

func (n Niveau) String() string {
	switch n {
	case NiveauIdentiques:
		return "identiques"
	case NiveauCertaines:
		return "certaines"
	case NiveauProbables:
		return "probables"
	case NiveauTout:
		return "tout"
	default:
		return "?"
	}
}

// ParseNiveau décode l'option --fusionner.
func ParseNiveau(s string) (Niveau, error) {
	switch s {
	case "identiques":
		return NiveauIdentiques, nil
	case "certaines":
		return NiveauCertaines, nil
	case "probables":
		return NiveauProbables, nil
	case "tout":
		return NiveauTout, nil
	default:
		return 0, fmt.Errorf("niveau de fusion inconnu : %q (attendu identiques|certaines|probables|tout)", s)
	}
}

func (n Niveau) accepte(c Classe) bool {
	switch n {
	case NiveauCertaines:
		return c == Certaine
	case NiveauProbables:
		return c == Certaine || c == Probable
	case NiveauTout:
		return true
	default: // NiveauIdentiques
		return false
	}
}

// Appariement propose que XrefBase (dans `base`) et XrefApport (dans `apport`)
// désignent la même personne. Criteres explique ce qui a compté pour le score ;
// Conflits liste les faits qui s'y opposent (retranchés du score, jamais tranchés
// automatiquement). Force indique une ancre fournie par l'utilisateur (voir
// forcemerge/PlanForce) plutôt que détectée par contenu ou par score — jamais remise
// en cause par l'appariement automatique.
type Appariement struct {
	XrefBase   string   `json:"xref_base"`
	XrefApport string   `json:"xref_apport"`
	NomBase    string   `json:"nom_base"`
	NomApport  string   `json:"nom_apport"`
	Score      int      `json:"score"`
	Classe     Classe   `json:"classe"`
	Criteres   []string `json:"criteres,omitempty"`
	Conflits   []string `json:"conflits,omitempty"`
	Force      bool     `json:"force,omitempty"`
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
	Collisions           Collisions    `json:"collisions"`
	Niveau               string        `json:"niveau"`
	Identiques           int           `json:"identiques"`  // réutilisés sans aucune modification
	Completees           int           `json:"completees"`  // fiches existantes enrichies de lignes complémentaires
	Nouveaux             int           `json:"nouveaux"`    // enregistrements insérés (add_record)
	Renumerotes          int           `json:"renumerotes"` // parmi les nouveaux, ceux dont le xref a dû changer (collision)
	ConflitsNonAppliques []string      `json:"conflits_non_appliques,omitempty"`
	Appariements         []Appariement `json:"appariements"` // triés par score décroissant, tous niveaux confondus
	NouveauxApresMerge   []string      `json:"nouveaux_apres_merge,omitempty"`
	Verdict              string        `json:"verdict"`
}

// Analyser compare base et apport et produit le rapport complet. N'écrit rien. forces
// (voir preparer) est nil pour une analyse ordinaire ; forcemerge y passe ses ancres.
func Analyser(base, apport *gedcom.Gedcom, niveau Niveau, forces map[string]string) *Analyse {
	f := preparer(base, apport, niveau, forces)
	collisions := detecterCollisions(base, apport)

	seuils := config.Defauts()
	apportTraduit := apport.Retraduire(f.table)
	avant := unionMessages(toutesRegles(base, seuils), toutesRegles(apportTraduit, seuils))

	fusionSimulee := base.Retraduire(nil)
	// Sonde interne, jamais écrite : préserverConflits=false ici n'affecte pas le
	// résultat (NOTE est inerte pour toutes les règles), et reste indépendant de ce que
	// PlanForce écrira réellement pour le mode "ancres".
	_ = planDepuis(f, "", niveau, false).Appliquer(fusionSimulee) // invariants de preparer/allouer garantissent l'applicabilité
	apres := toutesRegles(fusionSimulee, seuils)
	nouveaux := messagesNonPresents(apres, avant)

	a := &Analyse{
		Collisions:           collisions,
		Niveau:               niveau.String(),
		Identiques:           len(f.apparies) - len(f.completes),
		Completees:           len(f.completes),
		Nouveaux:             len(f.copies),
		Renumerotes:          len(f.renumerotes),
		ConflitsNonAppliques: messagesDeConflits(f.conflits),
		Appariements:         f.appariements,
		NouveauxApresMerge:   nouveaux,
	}
	a.Verdict = etablirVerdict(a)
	return a
}

func etablirVerdict(a *Analyse) string {
	switch {
	case len(a.NouveauxApresMerge) > 0:
		return fmt.Sprintf("à arbitrer : la fusion introduit %d signalement(s) nouveau(x)", len(a.NouveauxApresMerge))
	case len(a.ConflitsNonAppliques) > 0:
		return fmt.Sprintf("à arbitrer : %d bloc(s) en conflit non appliqué(s)", len(a.ConflitsNonAppliques))
	case a.Renumerotes > 0:
		return fmt.Sprintf("fusionnable après renumérotation de %d enregistrement(s) en collision", a.Renumerotes)
	default:
		return fmt.Sprintf("fusionnable tel quel — %d réutilisé(s), %d complété(s), %d nouveau(x)",
			a.Identiques, a.Completees, a.Nouveaux)
	}
}

func messagesDeConflits(conflits []Conflit) []string {
	out := make([]string, len(conflits))
	for i, c := range conflits {
		out[i] = c.Message
	}
	return out
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

// apparier calcule, pour chaque paire d'individus candidate (même patronyme normalisé),
// un score et une classe, puis ajoute les ancres forcées (forces, indexée xref d'apport
// -> xref de base — voir preparer) comme appariements Force:true, toujours inclus quel
// que soit leur score. Renvoie tous les candidats au-dessus du seuil d'affichage plus
// les ancres, indépendamment du niveau de fusion demandé — c'est affecterMeilleurs qui
// filtre et choisit une affectation 1-pour-1 pour le plan (les ancres, elles, sont déjà
// actées dans apparies par preparer avant même cette étape).
func apparier(base, apport *gedcom.Gedcom, forces map[string]string) []Appariement {
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

	// meilleur appariement provisoire par xref d'apport, pour la propagation par la
	// parenté (deuxième passe ci-dessous) — une ancre forcée y prend toujours le pas
	// sur un candidat scoré : c'est elle, jamais un score, qui doit propager aux
	// enfants/conjoints d'un individu forcé, même si l'ancre n'a jamais figuré dans
	// brut (patronymes différents, ex. une épouse rattachée par son nom de jeune fille).
	meilleur := map[string]Appariement{}
	for _, a := range brut {
		if cur, ok := meilleur[a.XrefApport]; !ok || a.Score > cur.Score {
			meilleur[a.XrefApport] = a
		}
	}
	var forcees []Appariement
	for xa, xb := range forces {
		ind1, _ := base.Get(xb)
		ind2, _ := apport.Get(xa)
		a := Appariement{
			XrefBase: xb, XrefApport: xa, Score: 100, Classe: Certaine, Force: true,
			Criteres: []string{"ancre forcée (mode miroir)"},
		}
		if ind1 != nil {
			a.NomBase = ind1.Nom()
		}
		if ind2 != nil {
			a.NomApport = ind2.Nom()
		}
		meilleur[xa] = a
		forcees = append(forcees, a)
	}

	var out []Appariement
	for _, a := range brut {
		if xb, force := forces[a.XrefApport]; force && xb == a.XrefBase {
			continue // déjà représenté par son entrée forcée (forcees) : pas de doublon
		}
		ind1, _ := base.Get(a.XrefBase)
		ind2, _ := apport.Get(a.XrefApport)
		bonus, crit := scorerParente(base, apport, ind1, ind2, meilleur)
		a.Score += bonus
		a.Criteres = append(a.Criteres, crit...)
		a.Classe = classer(a.Score, a.Conflits)
		if a.Score >= seuilAffichage {
			out = append(out, a)
		}
	}
	out = append(out, forcees...)
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
		score -= 40
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
		score -= 30
		conflits = append(conflits, fmt.Sprintf("naissance différente : %d vs %d", a1, a2))
	default:
		score += 5 // l'une des deux inconnue : ni pour ni contre
	}

	if lieu1, lieu2 := lieuNaissance(ind1), lieuNaissance(ind2); lieu1 != "" && lieu1 == lieu2 {
		score += 10
		criteres = append(criteres, "lieu de naissance identique")
	}

	if s1, s2 := ind1.Valeur("SEX"), ind2.Valeur("SEX"); s1 != "" && s2 != "" && s1 != s2 {
		score -= 50
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

// affecterMeilleurs choisit, parmi des appariements candidats qui peuvent partager un
// même xref des deux côtés, une affectation 1-pour-1 : trie par score décroissant et
// retient chaque paire tant que ni son xref de base ni son xref d'apport n'est déjà
// pris. Ne conserve que les paires dont la classe est acceptée par niveau.
func affecterMeilleurs(appariements []Appariement, niveau Niveau) map[string]string {
	tri := append([]Appariement{}, appariements...)
	sort.Slice(tri, func(i, j int) bool { return tri[i].Score > tri[j].Score })
	prisBase, prisApport := map[string]bool{}, map[string]bool{}
	table := map[string]string{}
	for _, a := range tri {
		if !niveau.accepte(a.Classe) {
			continue
		}
		if prisBase[a.XrefBase] || prisApport[a.XrefApport] {
			continue
		}
		table[a.XrefApport] = a.XrefBase
		prisBase[a.XrefBase], prisApport[a.XrefApport] = true, true
	}
	return table
}

// -------------------------------------------------------- identité par le contenu

var pointeurRe = regexp.MustCompile(`@([A-Za-z0-9_]+)@`)

// signature renvoie les lignes de l'enregistrement (hors ligne 0) avec tous les
// pointeurs "@XREF@" masqués — deux enregistrements de contenu identique ont la même
// signature quels que soient leurs xref respectifs, y compris ceux qu'ils pointent.
func signature(r *gedcom.Record) string {
	if len(r.Lignes) <= 1 {
		return ""
	}
	masquees := make([]string, len(r.Lignes)-1)
	for i, l := range r.Lignes[1:] {
		masquees[i] = pointeurRe.ReplaceAllString(l, "@@")
	}
	return strings.Join(masquees, "\n")
}

type cleSignature struct{ tag, sig string }

func grouperParSignature(g *gedcom.Gedcom) map[cleSignature][]*gedcom.Record {
	m := map[cleSignature][]*gedcom.Record{}
	for _, r := range g.Records {
		if r.Tag == "HEAD" || r.Tag == "TRLR" {
			continue
		}
		k := cleSignature{r.Tag, signature(r)}
		m[k] = append(m[k], r)
	}
	return m
}

// apparierContenu identifie les enregistrements de contenu identique entre base et
// apport, sans jamais se fier à leurs xref : regroupe par (Tag, signature masquée),
// apparie min(n, m) éléments dans l'ordre du fichier pour chaque groupe partagé — un
// groupe de cardinalité n:m se produit quand plusieurs enregistrements ont le même
// contenu visible (ex. plusieurs NOTE identiques) — puis CONFIRME chaque paire en
// traduisant les pointeurs réels de l'apport avec la table tentative complète et en
// vérifiant l'égalité avec la base : un groupe sans pointeur sortant (SOUR, NOTE,
// SUBM) se confirme trivialement ; un groupe avec pointeurs (INDI, FAM) ne se
// confirme que si les entités qu'il désigne concordent elles aussi.
//
// ponytail : une seule passe de confirmation (pas de point fixe itéré) — une paire
// rejetée ne peut que rendre d'autres paires rejetables, donc l'erreur penche
// toujours vers la duplication (visible, réversible), jamais vers la fusion
// silencieuse. Itérer jusqu'à convergence si un cas réel le montre nécessaire.
func apparierContenu(base, apport *gedcom.Gedcom) map[string]string {
	groupesBase := grouperParSignature(base)
	groupesApport := grouperParSignature(apport)

	type candidat struct{ base, apport *gedcom.Record }
	tentative := map[string]string{}
	var candidats []candidat
	for k, listeApport := range groupesApport {
		listeBase := groupesBase[k]
		n := len(listeApport)
		if len(listeBase) < n {
			n = len(listeBase)
		}
		for i := 0; i < n; i++ {
			candidats = append(candidats, candidat{listeBase[i], listeApport[i]})
			tentative[listeApport[i].Xref] = listeBase[i].Xref
		}
	}

	table := map[string]string{}
	for _, c := range candidats {
		traduites := gedcom.TraduireXrefs(c.apport.Lignes[1:], tentative)
		if strings.Join(traduites, "\n") == strings.Join(c.base.Lignes[1:], "\n") {
			table[c.apport.Xref] = c.base.Xref
		}
	}
	return table
}

// -------------------------------------------------------------- appariement de familles

// traduireXrefs traduit chaque xref de xrefs selon table, en abandonnant ceux sans
// correspondance connue (on ne peut rien affirmer sur un enfant non identifié).
func traduireXrefs(xrefs []string, table map[string]string) []string {
	var out []string
	for _, x := range xrefs {
		if t, ok := table[x]; ok {
			out = append(out, t)
		}
	}
	return out
}

func intersectionCount(a []string, b []string) int {
	bs := map[string]bool{}
	for _, x := range b {
		bs[x] = true
	}
	n := 0
	for _, x := range a {
		if bs[x] {
			n++
		}
	}
	return n
}

type candidatFamille struct {
	apport, base *gedcom.Record
	score        int
}

// apparierFamilles rattache une FAM d'apport (non déjà identifiée par contenu) à une
// FAM de base quand leurs HUSB/WIFE traduits sont compatibles (égaux, ou absents d'un
// côté — voir F0127/F0132 du cas réel, où le conjoint a disparu d'un des deux exports)
// et qu'elles partagent au moins un membre (HUSB, WIFE ou CHIL) déjà identifié.
// Affectation 1-pour-1 gloutonne sur tous les candidats, comme affecterMeilleurs.
func apparierFamilles(base, apport *gedcom.Gedcom, table map[string]string) map[string]string {
	baseFamilles := base.Familles()
	var candidats []candidatFamille
	for _, fa := range apport.Familles() {
		if _, deja := table[fa.Xref]; deja {
			continue
		}
		husbTA, husbOK := table[fa.Valeur("HUSB")]
		wifeTA, wifeOK := table[fa.Valeur("WIFE")]
		chilTA := traduireXrefs(fa.Valeurs("CHIL"), table)

		for _, fb := range baseFamilles {
			husbB, wifeB := fb.Valeur("HUSB"), fb.Valeur("WIFE")
			if husbOK && husbB != "" && husbTA != husbB {
				continue
			}
			if wifeOK && wifeB != "" && wifeTA != wifeB {
				continue
			}
			score := 0
			if husbOK && husbTA != "" && husbTA == husbB {
				score += 10
			}
			if wifeOK && wifeTA != "" && wifeTA == wifeB {
				score += 10
			}
			score += intersectionCount(chilTA, fb.Valeurs("CHIL"))
			if score > 0 {
				candidats = append(candidats, candidatFamille{fa, fb, score})
			}
		}
	}

	sort.Slice(candidats, func(i, j int) bool { return candidats[i].score > candidats[j].score })
	prisApport, prisBase := map[string]bool{}, map[string]bool{}
	resultat := map[string]string{}
	for _, c := range candidats {
		if prisApport[c.apport.Xref] || prisBase[c.base.Xref] {
			continue
		}
		resultat[c.apport.Xref] = c.base.Xref
		prisApport[c.apport.Xref], prisBase[c.base.Xref] = true, true
	}
	return resultat
}

// ------------------------------------------------------- renumérotation sur collision

// prefixeDepuis dérive un préfixe de nommage à partir d'un xref d'apport (ex.
// "I0116" -> "I") pour l'allocation d'un nouveau xref en cas de collision, avec un
// repli sur l'initiale du tag si le xref n'a pas de suffixe numérique.
var suffixeChiffresRe = regexp.MustCompile(`\d+$`)

func prefixeDepuis(xref, tag string) string {
	p := suffixeChiffresRe.ReplaceAllString(xref, "")
	if p == "" && tag != "" {
		p = strings.ToUpper(tag[:1])
	}
	return p
}

func xrefSuivant(xref, prefixe string) string {
	suffixe := strings.TrimPrefix(xref, prefixe)
	n, _ := strconv.Atoi(suffixe) // toujours numérique : format produit par gedcom.ProchainXref
	return fmt.Sprintf("%s%0*d", prefixe, len(suffixe), n+1)
}

// allouer complète, pour chaque xref d'apport non identifié (absent de apparies),
// une entrée de table : le xref d'origine s'il est libre dans la base et pas déjà
// alloué dans ce plan, sinon un nouveau via gedcom.ProchainXref. add_record ne
// vérifie pas lui-même qu'un xref est libre (voir patch.Operation.Appliquer) : c'est
// ici, une fois pour toutes, que la garantie est établie.
func allouer(base, apport *gedcom.Gedcom, apparies map[string]string) (table, renumerotes map[string]string) {
	table = make(map[string]string, len(apparies))
	for x, b := range apparies {
		table[x] = b
	}
	pris := map[string]bool{}
	for x := range base.ParXref() {
		pris[x] = true
	}
	renumerotes = map[string]string{}
	suivant := map[string]string{} // préfixe -> prochain candidat, avance à chaque allocation

	for _, r := range apport.Records {
		if r.Tag == "HEAD" || r.Tag == "TRLR" || r.Xref == "" {
			continue
		}
		if _, identifie := apparies[r.Xref]; identifie {
			continue
		}
		if !pris[r.Xref] {
			table[r.Xref] = r.Xref
			pris[r.Xref] = true
			continue
		}
		prefixe := prefixeDepuis(r.Xref, r.Tag)
		candidat, connu := suivant[prefixe]
		if !connu {
			candidat = base.ProchainXref(prefixe)
		}
		for pris[candidat] {
			candidat = xrefSuivant(candidat, prefixe)
		}
		table[r.Xref] = candidat
		renumerotes[r.Xref] = candidat
		pris[candidat] = true
		suivant[prefixe] = xrefSuivant(candidat, prefixe)
	}
	return table, renumerotes
}

// -------------------------------------------------------- union des lignes complémentaires

// tagsRepetables peuvent légitimement apparaître plusieurs fois sur un même
// enregistrement (plusieurs enfants, plusieurs notes, plusieurs sources...) : un bloc
// de ce tag absent de la base s'ajoute toujours. Un tag absent de tagsRepetables et
// déjà présent avec un contenu différent (ex. deux "1 MARR" de dates différentes) est
// un conflit de valeur, jamais tranché automatiquement.
var tagsRepetables = map[string]bool{
	"CHIL": true, "NOTE": true, "SOUR": true, "OBJE": true,
	"FAMS": true, "FAMC": true, "ASSO": true,
}

// tagsLiens sont les tags répétables dont la valeur (xref) identifie à elle seule la
// relation : un même FAMC/FAMS pointant deux fois vers la même famille est un doublon,
// même si ses sous-lignes diffèrent (PEDI, SOUR...) — contrairement à CHIL/SOUR/NOTE
// où deux blocs de même tag légitimement distincts (enfants, sources) sont fréquents.
var tagsLiens = map[string]bool{"FAMC": true, "FAMS": true}

// blocs découpe lignes (déjà hors ligne 0) en unités "ligne de niveau 1 + ses
// sous-lignes" — un fait GEDCOM ("1 MARR" + "2 DATE" + "2 PLAC"...) est une unité de
// comparaison, jamais une ligne isolée.
func blocs(lignes []string) [][]string {
	var out [][]string
	for _, l := range lignes {
		if d, ok := gedcom.Decoupe(l); ok && d.Niveau == 1 {
			out = append(out, []string{l})
			continue
		}
		if len(out) == 0 {
			continue
		}
		out[len(out)-1] = append(out[len(out)-1], l)
	}
	return out
}

// Conflit décrit un bloc mono-valué qui diverge entre baseRec et apportRec (ex. deux
// dates de mariage différentes) : Message est le texte du rapport (voir
// Analyse.ConflitsNonAppliques, inchangé) ; NoteLignes reformule la même information
// en bloc "1 NOTE ..." prêt pour add_lines, pour qu'aucune information de l'apport ne
// disparaisse silencieusement même quand le fait lui-même n'est pas retenu comme
// valeur — utilisé uniquement par PlanForce (forcemerge écrit directement, sans étape
// humaine de relecture avant écriture) ; Plan (automerge) l'ignore et laisse le
// conflit à l'arbitrage humain, comme avant.
type Conflit struct {
	XrefBase   string
	Message    string
	NoteLignes []string
}

// lignesAAjouter compare, bloc par bloc, apportRec (déjà traduit selon table) à
// baseRec : ajouts est ce qui manque à la base (tag répétable, ou tag totalement
// absent de la base) ; conflits est ce qui diverge sur un tag mono-valué déjà présent.
func lignesAAjouter(baseRec, apportRec *gedcom.Record, table map[string]string) (ajouts []string, conflits []Conflit) {
	baseBlocs := blocs(baseRec.Lignes[1:])
	baseSet := map[string]bool{}
	baseTags := map[string]bool{}
	baseLiens := map[string]bool{} // "TAG\nvaleur" pour FAMC/FAMS : un même lien ne se répète pas
	for _, b := range baseBlocs {
		baseSet[strings.Join(b, "\n")] = true
		if d, ok := gedcom.Decoupe(b[0]); ok {
			baseTags[d.Tag] = true
			if tagsLiens[d.Tag] {
				baseLiens[d.Tag+"\n"+d.Valeur] = true
			}
		}
	}

	apportBlocs := blocs(gedcom.TraduireXrefs(apportRec.Lignes[1:], table))
	for _, b := range apportBlocs {
		d, ok := gedcom.Decoupe(b[0])
		if !ok || d.Tag == "CHAN" {
			continue
		}
		if baseSet[strings.Join(b, "\n")] {
			continue
		}
		// FAMC/FAMS : même famille déjà liée (même xref) — pas de second bloc
		// pour cette même relation même si ses sous-lignes diffèrent (PEDI, SOUR...).
		if tagsLiens[d.Tag] && baseLiens[d.Tag+"\n"+d.Valeur] {
			continue
		}
		if tagsRepetables[d.Tag] || !baseTags[d.Tag] {
			ajouts = append(ajouts, b...)
			continue
		}
		texte := fmt.Sprintf("forcemerge : valeur alternative de l'apport non retenue automatiquement pour %s — %s",
			d.Tag, strings.Join(b, " / "))
		conflits = append(conflits, Conflit{
			XrefBase:   baseRec.Xref,
			Message:    fmt.Sprintf("%s : %s divergent (base garde le sien, non appliqué)", baseRec.Xref, d.Tag),
			NoteLignes: gedcom.EnligneNote(1, texte),
		})
	}
	return ajouts, conflits
}

// -------------------------------------------------------------------------- fusion

// fusion est le résultat de preparer : tout ce qu'il faut pour produire aussi bien le
// rapport (Analyser) que le correctif (Plan), à partir du même calcul — les deux ne
// peuvent donc jamais diverger l'un de l'autre.
type fusion struct {
	apparies     map[string]string   // xref apport -> xref base, identité confirmée (contenu, individu ou famille)
	table        map[string]string   // xref apport -> xref final (identifié, conservé ou nouveau) : couvre tout apport
	completes    map[string][]string // xref base -> lignes complémentaires à ajouter (add_lines)
	conflits     []Conflit           // blocs divergents, jamais appliqués comme valeur (voir Conflit)
	copies       []*gedcom.Record    // enregistrements d'apport traduits à insérer (add_record)
	renumerotes  map[string]string   // xref apport -> nouveau xref, uniquement en cas de collision réelle
	appariements []Appariement       // scoring individus complet (tous niveaux), pour le rapport
}

// preparer accepte forces (xref d'apport -> xref de base, précondition : les deux
// doivent exister dans base/apport respectivement — non revérifié ici, c'est la
// responsabilité de l'appelant, voir cmd_forcemerge.go) pour le mode "ancres" de
// forcemerge ; nil pour le mode contenu+score ordinaire (Analyser/Plan).
func preparer(base, apport *gedcom.Gedcom, niveau Niveau, forces map[string]string) *fusion {
	apparies := apparierContenu(base, apport)
	prisBase := map[string]bool{}
	for _, xb := range apparies {
		prisBase[xb] = true
	}

	// Ancres forcées : actées avant tout appariement automatique, jamais remises en
	// cause par lui (les boucles suivantes sautent déjà tout xa/xb déjà pris).
	for xa, xb := range forces {
		apparies[xa] = xb
		prisBase[xb] = true
	}

	scored := apparier(base, apport, forces)
	for xa, xb := range affecterMeilleurs(scored, niveau) {
		if _, deja := apparies[xa]; deja || prisBase[xb] {
			continue
		}
		apparies[xa] = xb
		prisBase[xb] = true
	}

	if niveau > NiveauIdentiques {
		for xa, xb := range apparierFamilles(base, apport, apparies) {
			if _, deja := apparies[xa]; deja || prisBase[xb] {
				continue
			}
			apparies[xa] = xb
			prisBase[xb] = true
		}
	}

	table, renumerotes := allouer(base, apport, apparies)

	f := &fusion{
		apparies: apparies, table: table, renumerotes: renumerotes,
		completes: map[string][]string{}, appariements: scored,
	}

	for _, r := range apport.Records {
		if r.Tag == "HEAD" || r.Tag == "TRLR" {
			continue
		}
		if xb, identifie := apparies[r.Xref]; identifie {
			baseRec, _ := base.Get(xb)
			ajouts, conflits := lignesAAjouter(baseRec, r, table)
			if len(ajouts) > 0 {
				f.completes[xb] = append(f.completes[xb], ajouts...)
			}
			f.conflits = append(f.conflits, conflits...)
			continue
		}
		f.copies = append(f.copies, gedcom.NewRecord(gedcom.TraduireXrefs(r.Lignes, table)))
	}
	return f
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

// planDepuis construit le correctif à partir de f. preserverConflits ne change jamais
// QUELLE valeur est retenue (la base garde toujours la sienne sur un conflit) — il
// décide seulement si la valeur alternative de l'apport est, en plus, préservée sous
// forme de NOTE (voir Conflit.NoteLignes) plutôt que simplement listée dans le
// rapport. false pour Plan (automerge : l'humain relit et arbitre lui-même via
// apply) ; true pour PlanForce (forcemerge : aucune étape de relecture avant
// écriture, donc rien de ce qui existe dans un des deux sources ne doit disparaître
// silencieusement de dst.ged).
func planDepuis(f *fusion, cheminBaseAffiche string, niveau Niveau, preserverConflits bool) *patch.Correctif {
	var ops []patch.Operation
	for _, r := range f.copies {
		ops = append(ops, patch.Operation{Op: "add_record", Lignes: append([]string{}, r.Lignes...)})
	}
	var xrefsCompletes []string
	for xb := range f.completes {
		xrefsCompletes = append(xrefsCompletes, xb)
	}
	sort.Strings(xrefsCompletes)
	for _, xb := range xrefsCompletes {
		ops = append(ops, patch.Operation{Op: "add_lines", Xref: xb, Lignes: append([]string{}, f.completes[xb]...)})
	}
	if preserverConflits {
		for _, c := range f.conflits {
			ops = append(ops, patch.Operation{Op: "add_lines", Xref: c.XrefBase, Lignes: append([]string{}, c.NoteLignes...)})
		}
	}

	origine, ancres := "automerge --analyse", ""
	if n := nAncresForcees(f.appariements); n > 0 {
		origine = "forcemerge"
		ancres = fmt.Sprintf(" (dont %d ancre(s) forcée(s))", n)
	}
	conflitsTexte := fmt.Sprintf("%d bloc(s) en conflit non appliqué(s) : à arbitrer manuellement.", len(f.conflits))
	if preserverConflits && len(f.conflits) > 0 {
		conflitsTexte = fmt.Sprintf("%d bloc(s) en conflit : la base garde sa valeur, l'alternative de l'apport est "+
			"préservée en NOTE sur chaque fiche concernée.", len(f.conflits))
	}
	return &patch.Correctif{
		Cible: cheminBaseAffiche,
		Justification: fmt.Sprintf(
			"Plan de fusion généré par `%s` (niveau %s) — réutilise %d enregistrement(s) déjà "+
				"identique(s)%s, complète %d fiche(s) existante(s), insère %d enregistrement(s) nouveau(x) "+
				"(dont %d renuméroté(s) pour collision de xref). %s",
			origine, niveau, len(f.apparies)-len(f.completes), ancres, len(f.completes), len(f.copies),
			len(f.renumerotes), conflitsTexte),
		Operations: ops,
	}
}

func nAncresForcees(appariements []Appariement) int {
	n := 0
	for _, a := range appariements {
		if a.Force {
			n++
		}
	}
	return n
}

// Plan construit le correctif déclaratif qui réaliserait la fusion au niveau demandé :
// réutilise tout ce qui est déjà identique dans la base, complète les fiches
// appariées avec les lignes qui leur manquent, et insère le reste (renumérotant
// seulement en cas de collision réelle de xref). Un appariement au-delà du niveau
// choisi reste visible au rapport (voir Analyser) mais n'entre jamais dans ce plan —
// c'est un jugement humain, à faire à la lecture des appariements "à examiner".
func Plan(base, apport *gedcom.Gedcom, cheminBaseAffiche string, niveau Niveau) *patch.Correctif {
	return planDepuis(preparer(base, apport, niveau, nil), cheminBaseAffiche, niveau, false)
}

// PlanForce construit le correctif de fusion comme Plan, mais en partant d'ancres
// fournies par l'utilisateur (forces, xref d'apport -> xref de base — voir preparer)
// plutôt que du seul contenu : ces ancres ne sont jamais remises en cause par
// l'appariement automatique (contenu, score, parenté), qui continue de tourner autour
// d'elles au niveau demandé — c'est le moteur du mode "ancres" de forcemerge. Un
// conflit de valeur préserve systématiquement l'alternative de l'apport en NOTE (voir
// planDepuis) : forcemerge écrit directement, sans étape humaine de relecture avant
// écriture, donc rien de ce qui existe dans un des deux sources ne doit disparaître.
func PlanForce(base, apport *gedcom.Gedcom, forces map[string]string, cheminBaseAffiche string, niveau Niveau) *patch.Correctif {
	return planDepuis(preparer(base, apport, niveau, forces), cheminBaseAffiche, niveau, true)
}

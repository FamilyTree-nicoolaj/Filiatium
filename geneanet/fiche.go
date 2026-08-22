// Package geneanet transforme la page HTML d'une fiche individuelle Geneanet en un
// arbre GEDCOM, en résolvant automatiquement les personnes qui se recoupent entre
// plusieurs fiches (voir builder.go). Ce fichier porte le modèle de données (Fiche et
// ses types associés) et les quelques helpers texte partagés entre plusieurs sections
// (années, nom GEDCOM...) — le parsing HTML lui-même vit dans html.go.
package geneanet

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Personne est une mention d'individu telle que rencontrée sur une fiche : un nom et,
// le plus souvent, seulement des années (le détail complet — date/lieu — n'est connu
// que pour le sujet propre d'une fiche, voir Fiche.Naissance/Deces).
type Personne struct {
	Nom                  string
	Sexe                 string // "M", "F", ou "" si inconnu
	Naissance, Deces     int
	OkNaissance, OkDeces bool

	// Champs utilisés seulement par le bloc "Grands-parents, oncles et tantes"
	// (voir GrandParentGroupe) — zéro/faux partout ailleurs, sans effet.
	MariageAnnee    int // mariage du grand-parent, ou de l'oncle/la tante
	OkMariage       bool
	SansDescendance bool // Geneanet n'affiche aucun enfant pour cette personne
}

// Evenement est une date/lieu complets, connus seulement pour le sujet d'une fiche.
type Evenement struct {
	Date string // date GEDCOM, "" si non reconnue
	Lieu string
}

// Union est une union et ses enfants. Note est le texte de la section "Notes
// concernant l'union" associée, "" si absente.
type Union struct {
	Conjoint Personne
	Mariage  Evenement // "" si date/lieu non précisés
	Enfants  []Personne
	Note     string
}

// DemiFratrieGroupe est le contenu d'un bloc "Du côté de <parent commun>".
type DemiFratrieGroupe struct {
	ParentCommun Personne
	Unions       []Union
}

// GrandParentGroupe est le contenu d'un bloc "Grands parents paternels/maternels,
// oncles et tantes" : un couple de grands-parents et tous leurs autres enfants — le
// parent du sujet lui-même n'y figure PAS (Geneanet l'omet, il apparaît déjà via
// Fiche.Parents), voir builder.go. Enfants reste nil quand la fiche source ne montre
// que l'arbre à 4 grands-parents (sans la liste oncles/tantes, qui nécessite une vue
// Geneanet différente) — jamais deviné.
type GrandParentGroupe struct {
	GrandPere, GrandMere Personne // position fixe, comme Fiche.Parents
	Enfants              []Personne
}

// Source est une puce de la section "Sources" : "Label: texte" ou juste "texte".
type Source struct {
	Label string
	Texte string
}

// Profession est la puce non datée de la liste naissance/décès.
type Profession struct {
	Intitule string
	Lieu     string
}

// Fiche est le contenu structuré d'une fiche individuelle Geneanet.
type Fiche struct {
	Sujet       Personne
	Naissance   Evenement
	Deces       Evenement
	Profession  *Profession
	Parents     [2]Personne // [0] = père, [1] = mère ; Nom == "" si absent
	Unions      []Union
	Fratrie     []Personne // germains, y compris le sujet lui-même (voir builder.go)
	DemiFratrie []DemiFratrieGroupe
	Sources     []Source
	Notes       []string // texte de la section "Notes" (individuelle), une entrée par note

	// nil si la section correspondante est absente de la fiche (comme Profession).
	GrandsParentsPaternels, GrandsParentsMaternels *GrandParentGroupe
}

var (
	reNaissDeces = regexp.MustCompile(`(?i)^(\S+)\s+le\s+(.+?)\s*\((.+?)\)\s*-\s*(.+)$`)
	reAgeSuffix  = regexp.MustCompile(`(?i),?\s*[aà]\s*l['’]?\s*[aâ]ge.*$`)

	reAvecSplit     = regexp.MustCompile(`(?i),?\s*avec\s+`)
	reMariagePrefix = regexp.MustCompile(`(?i)^(\S+)\s+le\s+(.+?)\s*\((.+?)\)\s*,\s*(.+)$`)
	reDontSuffix    = regexp.MustCompile(`(?i)\s+dont\s*$`)

	// Le suffixe "-" (sans second millésime) marque une naissance connue sans décès
	// enregistré ("1789-") — même sens qu'un millésime seul, juste une convention
	// d'affichage différente ; "ca "/"vers " marque une date approximative, ignoré.
	reAnneeDeuxRe      = regexp.MustCompile(`(?i)\s+(?:ca\.?\s+|vers\s+)?(\d{4})-(\d{4})\s*$`)
	reAnneeUneRe       = regexp.MustCompile(`(?i)\s+(?:ca\.?\s+|vers\s+)?(\d{4})-?\s*$`)
	reAnneeDecesSeulRe = regexp.MustCompile(`\s+[†+]\s*(\d{4})\s*$`)
	// "+"/"†" en fin de ligne SANS millésime (Geneanet : décès connu, date inconnue) :
	// retiré comme bruit plutôt que gardé comme dernier mot du nom. Le décès reste
	// simplement non daté (OkDeces=false).
	reMarqueurDecesSansAnnee = regexp.MustCompile(`\s+[†+]\s*$|\s+t\s*$`)

	// Marqueurs du bloc "Grands-parents, oncles et tantes" : mariage du couple de
	// grands-parents, ou mariage connu d'un oncle/une tante sans détail du conjoint.
	reMarqueurMariage         = regexp.MustCompile(`(?i)[⚭⊖]\s*\((\d{4})\)\s*$`)
	reMarqueurSansDescendance = regexp.MustCompile(`✂\s*$`)
)

// parseBulletNaissanceDeces reconnaît "Né(e)/Décédé(e) le <date> (<jour>) - <lieu>".
func (f *Fiche) parseBulletNaissanceDeces(contenu string) bool {
	m := reNaissDeces.FindStringSubmatch(contenu)
	if m == nil {
		return false
	}
	mot, dateTexte, reste := m[1], m[2], m[4]
	dateG, _ := DateGedcom(dateTexte)
	lieu := strings.TrimSpace(reAgeSuffix.ReplaceAllString(reste, ""))

	switch unicode.ToLower([]rune(mot)[0]) {
	case 'n':
		f.Naissance = Evenement{Date: dateG, Lieu: lieu}
	case 'd':
		f.Deces = Evenement{Date: dateG, Lieu: lieu}
	default:
		return false
	}
	return true
}

// parseOccupation traite une puce de la liste naissance/décès qui n'est ni une
// naissance ni un décès ("Laboureur, à Hasnon (Nord)") comme une profession —
// intitulé avant la première virgule, lieu après si présent. Une seule retenue (v1).
func (f *Fiche) parseOccupation(contenu string) {
	if f.Profession != nil {
		return
	}
	intitule, lieu := contenu, ""
	if idx := strings.Index(contenu, ","); idx >= 0 {
		intitule, lieu = strings.TrimSpace(contenu[:idx]), strings.TrimSpace(contenu[idx+1:])
	}
	f.Profession = &Profession{Intitule: intitule, Lieu: lieu}
}

// parseUnionBullet traite "Marié(e) le <date> (<jour>), <lieu>, avec <conjoint> dont".
// La date/le lieu de mariage sont optionnels : absents si le préfixe ne correspond
// pas (union sans date connue).
func parseUnionBullet(contenu string) Union {
	contenu = reDontSuffix.ReplaceAllString(contenu, "")
	var gauche, droite string
	if idx := reAvecSplit.FindStringIndex(contenu); idx != nil {
		gauche, droite = contenu[:idx[0]], contenu[idx[1]:]
	} else {
		droite = contenu
	}
	var ev Evenement
	if m := reMariagePrefix.FindStringSubmatch(gauche); m != nil {
		dateG, _ := DateGedcom(m[2])
		ev = Evenement{Date: dateG, Lieu: strings.TrimRight(strings.TrimSpace(m[4]), ", ")}
	}
	nom, naissance, deces, okN, okD := nomEtAnnees(droite)
	conjoint := Personne{Nom: nom, Naissance: naissance, Deces: deces, OkNaissance: okN, OkDeces: okD}
	return Union{Conjoint: conjoint, Mariage: ev}
}

// parseSource découpe "Label: texte" ; "texte" seul si aucun label.
func parseSource(l string) Source {
	if idx := strings.Index(l, ": "); idx >= 0 {
		return Source{Label: strings.TrimSpace(l[:idx]), Texte: strings.TrimSpace(l[idx+2:])}
	}
	return Source{Texte: l}
}

// nomEtAnnees sépare un nom de son suffixe d'années : "X 1826-1878" (naissance et
// décès), "X 1755" (naissance seule), "X †1805" (décès seul), ou "X" (aucune date).
func nomEtAnnees(ligne string) (nom string, naissance, deces int, okNaissance, okDeces bool) {
	ligne = strings.TrimSpace(ligne)
	if m := reAnneeDecesSeulRe.FindStringSubmatchIndex(ligne); m != nil {
		deces, _ = strconv.Atoi(ligne[m[2]:m[3]])
		okDeces = true
		return strings.TrimSpace(ligne[:m[0]]), naissance, deces, okNaissance, okDeces
	}
	if m := reAnneeDeuxRe.FindStringSubmatchIndex(ligne); m != nil {
		naissance, _ = strconv.Atoi(ligne[m[2]:m[3]])
		deces, _ = strconv.Atoi(ligne[m[4]:m[5]])
		okNaissance, okDeces = true, true
		return strings.TrimSpace(ligne[:m[0]]), naissance, deces, okNaissance, okDeces
	}
	if m := reAnneeUneRe.FindStringSubmatchIndex(ligne); m != nil {
		naissance, _ = strconv.Atoi(ligne[m[2]:m[3]])
		okNaissance = true
		return strings.TrimSpace(ligne[:m[0]]), naissance, deces, okNaissance, okDeces
	}
	ligne = strings.TrimSpace(reMarqueurDecesSansAnnee.ReplaceAllString(ligne, ""))
	return ligne, naissance, deces, okNaissance, okDeces
}

// nomGedcom convertit un nom affiché en NAME GEDCOM ("Victoire LOUIS" -> "Victoire
// /LOUIS/") : le patronyme est la plus longue suite finale de mots tout en
// majuscules (rendu Geneanet habituel pour certains blocs) ; si aucune, le dernier
// mot sert de patronyme par défaut — convention la plus courante pour un nom
// français, imparfaite pour un patronyme composé mais un repli raisonnable.
func nomGedcom(nomAffiche string) string {
	mots := strings.Fields(nomAffiche)
	if len(mots) == 0 {
		return nomAffiche
	}
	if len(mots) == 1 {
		return "/" + mots[0] + "/"
	}
	fin := len(mots)
	for fin > 0 && estToutMajuscule(mots[fin-1]) {
		fin--
	}
	if fin == len(mots) {
		fin = len(mots) - 1
	}
	prenoms := strings.Join(mots[:fin], " ")
	patronyme := strings.Join(mots[fin:], " ")
	return strings.TrimSpace(prenoms + " /" + patronyme + "/")
}

func estToutMajuscule(mot string) bool {
	uneMajuscule := false
	for _, r := range mot {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsUpper(r) {
			uneMajuscule = true
		}
	}
	return uneMajuscule
}

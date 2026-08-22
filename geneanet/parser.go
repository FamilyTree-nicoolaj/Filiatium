// Package geneanet transforme le texte OCR'd d'une fiche individuelle Geneanet
// (parents, union(s)/enfants, frères et sœurs, demi-frères et demi-sœurs, sources) en
// un arbre GEDCOM, en résolvant automatiquement les personnes qui se recoupent entre
// plusieurs fiches (voir builder.go).
package geneanet

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
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
	MariageAnnee    int // "⚭ (année)" (grand-parent) ou "⊖ (année)" (oncle/tante)
	OkMariage       bool
	SansDescendance bool // "✂" : Geneanet n'affiche aucun enfant pour cette personne
}

// Evenement est une date/lieu complets, connus seulement pour le sujet d'une fiche.
type Evenement struct {
	Date string // date GEDCOM, "" si non reconnue
	Lieu string
}

// Union est une union et ses enfants, ou un groupe "avec X" de demi-fratrie.
type Union struct {
	Conjoint Personne
	Mariage  Evenement // "" si date/lieu non précisés
	Enfants  []Personne
}

// DemiFratrieGroupe est le contenu d'un bloc "Du côté de <parent commun>".
type DemiFratrieGroupe struct {
	ParentCommun Personne
	Unions       []Union
}

// GrandParentGroupe est le contenu d'un bloc "Grands parents paternels/maternels,
// oncles et tantes" : un couple de grands-parents et tous leurs autres enfants — le
// parent du sujet lui-même n'y figure PAS (Geneanet l'omet, il apparaît déjà via
// Fiche.Parents), voir builder.go.
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

	// nil si la section correspondante est absente de la fiche (comme Profession).
	GrandsParentsPaternels, GrandsParentsMaternels *GrandParentGroupe
}

var (
	reHeadParents     = regexp.MustCompile(`(?i)^parents\s*$`)
	reHeadUnion       = regexp.MustCompile(`(?i)^union`)
	reHeadDemi        = regexp.MustCompile(`(?i)^demi`)
	reHeadFratrie     = regexp.MustCompile(`(?i)^fr[eè]res\s+et\s+s`)
	reHeadSources     = regexp.MustCompile(`(?i)^sources\s*$`)
	reHeadGPPaternels = regexp.MustCompile(`(?i)^grands?[\s-]*parents?\s+paternels`)
	reHeadGPMaternels = regexp.MustCompile(`(?i)^grands?[\s-]*parents?\s+maternels`)

	reNaissDeces = regexp.MustCompile(`(?i)^(\S+)\s+le\s+(.+?)\s*\((.+?)\)\s*-\s*(.+)$`)
	reAgeSuffix  = regexp.MustCompile(`(?i),?\s*[aà]\s*l['’]?\s*[aâ]ge.*$`)

	reAvecSplit     = regexp.MustCompile(`(?i),?\s*avec\s+`)
	reMariagePrefix = regexp.MustCompile(`(?i)^(\S+)\s+le\s+(.+?)\s*\((.+?)\)\s*,\s*(.+)$`)
	reDontSuffix    = regexp.MustCompile(`(?i)\s+dont\s*$`)
	reAvecPrefix    = regexp.MustCompile(`(?i)^avec\s+`)
	reDuCoteDe      = regexp.MustCompile(`(?i)^du\s*c[oô]t[eé]\s+de\s+(.+)$`)

	// Le suffixe "-" (sans second millésime) marque une naissance connue sans décès
	// enregistré ("1789-") — même sens qu'un millésime seul, juste une convention
	// d'affichage différente ; "ca "/"vers " marque une date approximative, ignoré.
	reAnneeDeuxRe      = regexp.MustCompile(`\s+(\d{4})-(\d{4})\s*$`)
	reAnneeUneRe       = regexp.MustCompile(`(?i)\s+(?:ca\.?\s+|vers\s+)?(\d{4})-?\s*$`)
	reAnneeDecesSeulRe = regexp.MustCompile(`\s+†\s*(\d{4})\s*$`)

	// Marqueurs du bloc "Grands-parents, oncles et tantes" : "⚭ (année)" (mariage du
	// couple de grands-parents) ou "⊖ (année)" (mariage connu d'un oncle/une tante,
	// conjoint non précisé), et "✂" (sans descendance connue).
	reMarqueurMariage         = regexp.MustCompile(`(?i)[⚭⊖]\s*\((\d{4})\)\s*$`)
	reMarqueurSansDescendance = regexp.MustCompile(`✂\s*$`)
)

// entete reconnaît un intitulé de section, insensible à la casse/aux accents — jamais
// à un ordre supposé (voir Parse).
func entete(l string) string {
	switch {
	case reHeadParents.MatchString(l):
		return "parents"
	case reHeadUnion.MatchString(l):
		return "union"
	case reHeadDemi.MatchString(l):
		return "demifratrie"
	case reHeadFratrie.MatchString(l):
		return "fratrie"
	case reHeadSources.MatchString(l):
		return "sources"
	case reHeadGPPaternels.MatchString(l):
		return "gpPaternels"
	case reHeadGPMaternels.MatchString(l):
		return "gpMaternels"
	}
	return ""
}

// Parse découpe le texte OCR'd d'une fiche en Fiche structurée. Une fiche dont le nom
// du sujet n'est pas reconnaissable en tête produit une erreur ; du bruit OCR
// ponctuel plus loin (puce non reconnue) est silencieusement ignoré plutôt que de
// faire échouer toute la fiche — voir le README pour ce choix de robustesse.
func Parse(texte string) (*Fiche, error) {
	var lignes []string
	for _, l := range strings.Split(texte, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			lignes = append(lignes, l)
		}
	}
	if len(lignes) == 0 {
		return nil, fmt.Errorf("fiche vide")
	}

	f := &Fiche{}
	nomSujet, sexe := peelGlyph(lignes[0])
	if nomSujet == "" {
		return nil, fmt.Errorf("nom du sujet introuvable en tête de fiche")
	}
	f.Sujet.Nom, f.Sujet.Sexe = nomSujet, sexe

	i := 1
	for i < len(lignes) && strings.HasPrefix(lignes[i], "- ") {
		contenu := strings.TrimPrefix(lignes[i], "- ")
		if !f.parseBulletNaissanceDeces(contenu) {
			f.parseOccupation(contenu)
		}
		i++
	}

	section := ""
	var demiGroupe *DemiFratrieGroupe
	var unionCourante *Union
	var gpGroupe *GrandParentGroupe
	for ; i < len(lignes); i++ {
		l := lignes[i]
		if sec := entete(l); sec != "" {
			section = sec
			unionCourante, demiGroupe, gpGroupe = nil, nil, nil
			switch sec {
			case "gpPaternels":
				f.GrandsParentsPaternels = &GrandParentGroupe{}
				gpGroupe = f.GrandsParentsPaternels
			case "gpMaternels":
				f.GrandsParentsMaternels = &GrandParentGroupe{}
				gpGroupe = f.GrandsParentsMaternels
			}
			continue
		}
		bullet := strings.HasPrefix(l, "- ")
		contenu := strings.TrimPrefix(l, "- ")

		switch section {
		case "parents":
			if !bullet {
				continue
			}
			p := parsePersonneLigne(contenu)
			switch {
			case f.Parents[0].Nom == "":
				f.Parents[0] = p
			case f.Parents[1].Nom == "":
				f.Parents[1] = p
			}
		case "union":
			if bullet {
				u := parseUnionBullet(contenu)
				f.Unions = append(f.Unions, u)
				unionCourante = &f.Unions[len(f.Unions)-1]
			} else if unionCourante != nil {
				unionCourante.Enfants = append(unionCourante.Enfants, parsePersonneLigne(contenu))
			}
		case "fratrie":
			if bullet {
				f.Fratrie = append(f.Fratrie, parsePersonneLigne(contenu))
			}
		case "demifratrie":
			switch {
			case !bullet && reDuCoteDe.MatchString(l):
				m := reDuCoteDe.FindStringSubmatch(l)
				f.DemiFratrie = append(f.DemiFratrie, DemiFratrieGroupe{ParentCommun: parsePersonneLigne(m[1])})
				demiGroupe = &f.DemiFratrie[len(f.DemiFratrie)-1]
			case bullet && reAvecPrefix.MatchString(contenu):
				if demiGroupe == nil {
					continue
				}
				demiGroupe.Unions = append(demiGroupe.Unions, parseAvecBullet(contenu))
			case !bullet && demiGroupe != nil && len(demiGroupe.Unions) > 0:
				u := &demiGroupe.Unions[len(demiGroupe.Unions)-1]
				u.Enfants = append(u.Enfants, parsePersonneLigne(contenu))
			}
		case "sources":
			if bullet {
				f.Sources = append(f.Sources, parseSource(contenu))
			}
		case "gpPaternels", "gpMaternels":
			if !bullet || gpGroupe == nil {
				continue
			}
			p := parsePersonneGP(contenu)
			switch {
			case gpGroupe.GrandPere.Nom == "":
				gpGroupe.GrandPere = p
			case gpGroupe.GrandMere.Nom == "":
				gpGroupe.GrandMere = p
			default:
				gpGroupe.Enfants = append(gpGroupe.Enfants, p)
			}
		}
	}

	if annee, ok := gedcom.Annee(f.Naissance.Date); ok {
		f.Sujet.Naissance, f.Sujet.OkNaissance = annee, true
	}
	if annee, ok := gedcom.Annee(f.Deces.Date); ok {
		f.Sujet.Deces, f.Sujet.OkDeces = annee, true
	}
	return f, nil
}

// parseBulletNaissanceDeces reconnaît "Né(e)/Décédé(e) le <date> (<jour>) - <lieu>" —
// le participe (dernière lettre 'e' non accentuée = féminin, 'é' = masculin, voir
// sexeDeParticipe) est le signal de sexe primaire, plus fiable qu'un glyphe ♂/♀ OCR'd.
func (f *Fiche) parseBulletNaissanceDeces(contenu string) bool {
	m := reNaissDeces.FindStringSubmatch(contenu)
	if m == nil {
		return false
	}
	mot, dateTexte, reste := m[1], m[2], m[4]
	if sexe := sexeDeParticipe(mot); sexe != "" {
		f.Sujet.Sexe = sexe
	}
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
// naissance ni un décès ("- Laboureur, à Hasnon (Nord)") comme une profession —
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
		ev = Evenement{Date: dateG, Lieu: strings.TrimSpace(m[4])}
	}
	return Union{Conjoint: parsePersonneLigne(droite), Mariage: ev}
}

// parseAvecBullet traite "avec <autre parent> dont" (demi-fratrie) — même conjoint
// qu'une union, sans date/lieu de mariage (jamais montrés sous cette forme groupée).
func parseAvecBullet(contenu string) Union {
	contenu = reDontSuffix.ReplaceAllString(contenu, "")
	contenu = reAvecPrefix.ReplaceAllString(contenu, "")
	return Union{Conjoint: parsePersonneLigne(contenu)}
}

// parseSource découpe "Label: texte" ; "texte" seul si aucun label.
func parseSource(l string) Source {
	if idx := strings.Index(l, ": "); idx >= 0 {
		return Source{Label: strings.TrimSpace(l[:idx]), Texte: strings.TrimSpace(l[idx+2:])}
	}
	return Source{Texte: l}
}

// parsePersonneLigne traite une mention "<glyphe?> <Nom> <années?>" — forme commune à
// Parents, Frères et sœurs, enfants d'union, et conjoint de demi-fratrie.
func parsePersonneLigne(l string) Personne {
	reste, sexe := peelGlyph(l)
	nom, naissance, deces, okN, okD := nomEtAnnees(reste)
	return Personne{Nom: nom, Sexe: sexe, Naissance: naissance, Deces: deces, OkNaissance: okN, OkDeces: okD}
}

// parsePersonneGP traite une ligne du bloc "Grands-parents, oncles et tantes" :
// mêmes "<glyphe?> <Nom> <années?>" que parsePersonneLigne, avec en plus les
// marqueurs "✂" (sans descendance) et "⚭"/"⊖ (année)" (mariage sans détail),
// possibles sur la ligne d'un grand-parent comme sur celle d'un oncle/une tante.
func parsePersonneGP(l string) Personne {
	l = strings.TrimSpace(l)
	sansDesc := reMarqueurSansDescendance.MatchString(l)
	if sansDesc {
		l = strings.TrimSpace(reMarqueurSansDescendance.ReplaceAllString(l, ""))
	}
	var mariageAnnee int
	var okMariage bool
	if m := reMarqueurMariage.FindStringSubmatch(l); m != nil {
		mariageAnnee, _ = strconv.Atoi(m[1])
		okMariage = true
		l = strings.TrimSpace(reMarqueurMariage.ReplaceAllString(l, ""))
	}
	p := parsePersonneLigne(l)
	p.MariageAnnee, p.OkMariage, p.SansDescendance = mariageAnnee, okMariage, sansDesc
	return p
}

// peelGlyph retire un glyphe ♂/♀ de tête s'il est présent — absent après OCR sur bien
// des captures, jamais requis.
func peelGlyph(s string) (reste, sexe string) {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, "♂"):
		return strings.TrimSpace(strings.TrimPrefix(s, "♂")), "M"
	case strings.HasPrefix(s, "♀"):
		return strings.TrimSpace(strings.TrimPrefix(s, "♀")), "F"
	}
	return s, ""
}

// sexeDeParticipe déduit le sexe d'un participe français par sa dernière lettre :
// "e" non accentué = féminin (Née, Décédée, Mariée), "é" = masculin (Né, Décédé,
// Marié) — vrai pour les trois participes rencontrés sur une fiche Geneanet.
func sexeDeParticipe(mot string) string {
	r := []rune(mot)
	if len(r) == 0 {
		return ""
	}
	switch r[len(r)-1] {
	case 'e', 'E':
		return "F"
	case 'é', 'É':
		return "M"
	}
	return ""
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
	return ligne, naissance, deces, okNaissance, okDeces
}

// nomGedcom convertit un nom affiché en NAME GEDCOM ("Victoire LOUIS" -> "Victoire
// /LOUIS/") : le patronyme est la plus longue suite finale de mots tout en
// majuscules (rendu Geneanet habituel) ; si aucune (certaines captures, notamment le
// bloc "Grands-parents, oncles et tantes", rendent tout en casse normale), le
// dernier mot sert de patronyme par défaut — convention la plus courante pour un nom
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

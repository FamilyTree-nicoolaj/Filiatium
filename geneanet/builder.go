package geneanet

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/FamilyTree-nicoolaj/filiatium/gedcom"
)

// Rapport résume ce que Construire a fait, pour affichage (voir cmd_import.go).
type Rapport struct {
	Individus int      `json:"individus"`
	Familles  int      `json:"familles"`
	Sources   int      `json:"sources"`
	Ambigus   []string `json:"ambigus,omitempty"`
}

// mention est une personne à résoudre (créer ou retrouver) — sujet d'une fiche
// (données riches : date/lieu complets) ou mention éparse ailleurs (années seules).
type mention struct {
	nom                              string
	sexe                             string
	naissance, deces                 int
	okNaissance, okDeces             bool
	dateNaissComplete, lieuNaissance string
	dateDecesComplete, lieuDeces     string
	occupation, occupationLieu       string
}

func mentionSujet(f *Fiche) mention {
	m := mention{
		nom: f.Sujet.Nom, sexe: f.Sujet.Sexe,
		naissance: f.Sujet.Naissance, deces: f.Sujet.Deces,
		okNaissance: f.Sujet.OkNaissance, okDeces: f.Sujet.OkDeces,
		dateNaissComplete: f.Naissance.Date, lieuNaissance: f.Naissance.Lieu,
		dateDecesComplete: f.Deces.Date, lieuDeces: f.Deces.Lieu,
	}
	if f.Profession != nil {
		m.occupation, m.occupationLieu = f.Profession.Intitule, f.Profession.Lieu
	}
	return m
}

func mentionDePersonne(p Personne) mention {
	return mention{
		nom: p.Nom, sexe: p.Sexe, naissance: p.Naissance, deces: p.Deces,
		okNaissance: p.OkNaissance, okDeces: p.OkDeces,
	}
}

// mentionsEparses énumère toute personne mentionnée sur une fiche hors de son propre
// sujet : parents, conjoint et enfants de chaque union, fratrie, demi-fratrie.
func mentionsEparses(f *Fiche) []mention {
	var out []mention
	ajouter := func(p Personne) {
		if p.Nom == "" {
			return
		}
		out = append(out, mentionDePersonne(p))
	}
	for _, p := range f.Parents {
		ajouter(p)
	}
	for _, u := range f.Unions {
		ajouter(u.Conjoint)
		for _, e := range u.Enfants {
			ajouter(e)
		}
	}
	for _, s := range f.Fratrie {
		ajouter(s)
	}
	for _, grp := range f.DemiFratrie {
		ajouter(grp.ParentCommun)
		for _, u := range grp.Unions {
			ajouter(u.Conjoint)
			for _, e := range u.Enfants {
				ajouter(e)
			}
		}
	}
	return out
}

// resoudre retrouve un individu déjà présent (trouverExistant) ou le crée. Appelée
// deux fois en passe 1 (sujets, puis mentions éparses — dans cet ordre, pour que la
// donnée la plus riche gagne toujours) et en passe 2 lors du câblage : à ce stade
// tout le monde existe déjà, donc c'est systématiquement une recherche, jamais une
// création — voir geneanet_test.go.
func resoudre(g *gedcom.Gedcom, m mention) (string, error) {
	if m.nom == "" {
		return "", fmt.Errorf("mention sans nom")
	}
	nomG := nomGedcom(m.nom)
	if xref, ok := trouverExistant(g, nomG, m); ok {
		return xref, nil
	}

	var lignes []string
	lignes = append(lignes, "1 NAME "+nomG)
	if m.sexe != "" {
		lignes = append(lignes, "1 SEX "+m.sexe)
	}
	if bloc := blocEvenement("BIRT", m.dateNaissComplete, m.okNaissance, m.naissance, m.lieuNaissance); bloc != nil {
		lignes = append(lignes, bloc...)
	}
	if bloc := blocEvenement("DEAT", m.dateDecesComplete, m.okDeces, m.deces, m.lieuDeces); bloc != nil {
		lignes = append(lignes, bloc...)
	}
	if m.occupation != "" {
		lignes = append(lignes, "1 OCCU "+m.occupation)
		if m.occupationLieu != "" {
			lignes = append(lignes, "2 PLAC "+m.occupationLieu)
		}
	}

	xref := g.ProchainXref("I")
	if _, err := g.AddIndividual(xref, lignes, ""); err != nil {
		return "", err
	}
	return xref, nil
}

func blocEvenement(tag, dateComplete string, ok bool, annee int, lieu string) []string {
	if dateComplete == "" && !ok {
		return nil
	}
	lignes := []string{"1 " + tag}
	switch {
	case dateComplete != "":
		lignes = append(lignes, "2 DATE "+dateComplete)
	case ok:
		lignes = append(lignes, "2 DATE "+strconv.Itoa(annee))
	}
	if lieu != "" {
		lignes = append(lignes, "2 PLAC "+lieu)
	}
	return lignes
}

// trouverExistant retrouve un individu déjà créé par cet import : même patronyme et
// prénom normalisés (gedcom.Normaliser, insensible casse/accents), et — quand les
// deux années de naissance sont connues — la MÊME année, jamais une simple fenêtre.
// add.ChercherHomonymes tolère ±3 ans exprès, pour qu'un humain confirme ou refuse
// (voir add.Requete.IgnorerHomonymes) ; ici la résolution est automatique et sans
// confirmation, donc une fenêtre large fusionnerait à tort deux germains distincts
// portant le même nom à un an d'écart (cas réel : un enfant mort en bas âge, puis un
// second baptisé du même nom l'année suivante — exactement le cas de "François Joseph
// BOUCHART" 1786-1787 et 1787-1870 sur la fiche B).
func trouverExistant(g *gedcom.Gedcom, nomGedcom string, m mention) (string, bool) {
	patronyme := gedcom.Normaliser(gedcom.PatronymeDeNom(nomGedcom))
	if patronyme == "" {
		return "", false
	}
	prenom := gedcom.Normaliser(gedcom.PrenomDeNom(nomGedcom))
	for _, ind := range g.Individus() {
		if gedcom.Normaliser(ind.Patronyme()) != patronyme {
			continue
		}
		if prenom != "" && gedcom.Normaliser(gedcom.PrenomDeNom(ind.Valeur("NAME"))) != prenom {
			continue
		}
		if m.okNaissance {
			if an, ok := gedcom.Annee(ind.Date("BIRT")); ok && an != m.naissance {
				continue
			}
		}
		return ind.Xref, true
	}
	return "", false
}

// Construire résout et câble toutes les fiches dans g : deux passes (voir resoudre),
// puis câblage fiche par fiche (parents+fratrie, unions+enfants, demi-fratrie,
// sources). auteur (peut être "") est attribué à une source Geneanet partagée, créée
// une fois, citée sur chaque enregistrement créé par cet import.
func Construire(g *gedcom.Gedcom, fiches []*Fiche, auteur string) (*Rapport, error) {
	rap := &Rapport{}
	if len(fiches) == 0 {
		return rap, nil
	}
	debutIndi, debutFam := len(g.Individus()), len(g.Familles())

	for _, f := range fiches {
		if _, err := resoudre(g, mentionSujet(f)); err != nil {
			return nil, fmt.Errorf("%s : %w", f.Sujet.Nom, err)
		}
	}
	for _, f := range fiches {
		for _, m := range mentionsEparses(f) {
			if _, err := resoudre(g, m); err != nil {
				return nil, fmt.Errorf("%s : %w", m.nom, err)
			}
		}
	}

	sourceGeneanet, err := g.AddSource(g.ProchainXref("S"), "Geneanet (import OCR filiatium)", auteur, "", "", "")
	if err != nil {
		return nil, err
	}
	sourcesXref := map[string]string{}

	for _, f := range fiches {
		if err := cablerFiche(g, f, sourcesXref, sourceGeneanet.Xref, rap); err != nil {
			return nil, fmt.Errorf("%s : %w", f.Sujet.Nom, err)
		}
	}

	rap.Individus = len(g.Individus()) - debutIndi
	rap.Familles = len(g.Familles()) - debutFam
	rap.Sources = len(sourcesXref) + 1
	return rap, nil
}

func cablerFiche(g *gedcom.Gedcom, f *Fiche, sourcesXref map[string]string, sourceGeneanet string, rap *Rapport) error {
	sujetXref, err := resoudre(g, mentionSujet(f))
	if err != nil {
		return err
	}
	sujet, _ := g.Get(sujetXref)
	sujet.AddCitation(sourceGeneanet, "")

	if err := cablerParentsEtFratrie(g, f, sujetXref, sourceGeneanet, rap); err != nil {
		return err
	}
	if err := cablerUnions(g, f, sujetXref, sourceGeneanet); err != nil {
		return err
	}
	if err := cablerDemiFratrie(g, f, sourceGeneanet); err != nil {
		return err
	}
	if err := cablerGrandsParents(g, f, sourceGeneanet); err != nil {
		return err
	}
	cablerSources(g, f, sujet, sourcesXref)
	return nil
}

func cablerParentsEtFratrie(g *gedcom.Gedcom, f *Fiche, sujetXref, sourceGeneanet string, rap *Rapport) error {
	pere, mere := f.Parents[0], f.Parents[1]
	if pere.Nom == "" && mere.Nom == "" {
		if len(f.Fratrie) > 0 {
			rap.Ambigus = append(rap.Ambigus, fmt.Sprintf(
				"%s : frères et sœurs listés sans section Parents, non câblés", f.Sujet.Nom))
		}
		return nil
	}

	var pereXref, mereXref string
	var err error
	if pere.Nom != "" {
		if pereXref, err = resoudre(g, mentionDePersonne(pere)); err != nil {
			return err
		}
		citerSiExiste(g, pereXref, sourceGeneanet)
	}
	if mere.Nom != "" {
		if mereXref, err = resoudre(g, mentionDePersonne(mere)); err != nil {
			return err
		}
		citerSiExiste(g, mereXref, sourceGeneanet)
	}

	fam, err := trouverOuCreerFamille(g, pereXref, mereXref)
	if err != nil {
		return err
	}
	cablerEnfant(g, fam, sujetXref)

	for _, s := range f.Fratrie {
		xref, err := resoudre(g, mentionDePersonne(s))
		if err != nil {
			return err
		}
		cablerEnfant(g, fam, xref)
		citerSiExiste(g, xref, sourceGeneanet)
	}
	return nil
}

func cablerUnions(g *gedcom.Gedcom, f *Fiche, sujetXref, sourceGeneanet string) error {
	for _, u := range f.Unions {
		conjointXref, err := resoudre(g, mentionDePersonne(u.Conjoint))
		if err != nil {
			return err
		}
		citerSiExiste(g, conjointXref, sourceGeneanet)

		fam, err := trouverOuCreerFamilleConjoint(g, sujetXref, conjointXref, f.Sujet.Sexe)
		if err != nil {
			return err
		}
		ajouterMariageSiAbsent(fam, u)

		for _, e := range u.Enfants {
			xref, err := resoudre(g, mentionDePersonne(e))
			if err != nil {
				return err
			}
			cablerEnfant(g, fam, xref)
			citerSiExiste(g, xref, sourceGeneanet)
		}
	}
	return nil
}

func cablerDemiFratrie(g *gedcom.Gedcom, f *Fiche, sourceGeneanet string) error {
	for _, grp := range f.DemiFratrie {
		parentXref, err := resoudre(g, mentionDePersonne(grp.ParentCommun))
		if err != nil {
			return err
		}
		citerSiExiste(g, parentXref, sourceGeneanet)
		for _, u := range grp.Unions {
			autreXref, err := resoudre(g, mentionDePersonne(u.Conjoint))
			if err != nil {
				return err
			}
			citerSiExiste(g, autreXref, sourceGeneanet)
			fam, err := trouverOuCreerFamilleConjoint(g, parentXref, autreXref, grp.ParentCommun.Sexe)
			if err != nil {
				return err
			}
			for _, e := range u.Enfants {
				xref, err := resoudre(g, mentionDePersonne(e))
				if err != nil {
					return err
				}
				cablerEnfant(g, fam, xref)
				citerSiExiste(g, xref, sourceGeneanet)
			}
		}
	}
	return nil
}

// cablerGrandsParents câble les blocs "Grands-parents paternels/maternels, oncles et
// tantes" (absents si nil, comme le reste des sections optionnelles).
func cablerGrandsParents(g *gedcom.Gedcom, f *Fiche, sourceGeneanet string) error {
	if f.GrandsParentsPaternels != nil {
		if err := cablerGroupeGrandsParents(g, f.GrandsParentsPaternels, f.Parents[0], sourceGeneanet); err != nil {
			return err
		}
	}
	if f.GrandsParentsMaternels != nil {
		if err := cablerGroupeGrandsParents(g, f.GrandsParentsMaternels, f.Parents[1], sourceGeneanet); err != nil {
			return err
		}
	}
	return nil
}

// cablerGroupeGrandsParents câble un couple de grands-parents et leurs enfants
// (oncles/tantes). parentDuSujet (déjà résolu par cablerParentsEtFratrie) est câblé
// explicitement comme enfant de ce couple bien qu'absent de grp.Enfants : Geneanet
// omet le parent du sujet de cette liste (il apparaît déjà via la section Parents),
// mais il EST bien un enfant de ce couple — même logique que le sujet toujours câblé
// sur sa propre FAM parentale, qu'il soit ou non relisté dans la Fratrie.
func cablerGroupeGrandsParents(g *gedcom.Gedcom, grp *GrandParentGroupe, parentDuSujet Personne, sourceGeneanet string) error {
	gpXref, err := resoudre(g, mentionDePersonne(grp.GrandPere))
	if err != nil {
		return err
	}
	citerSiExiste(g, gpXref, sourceGeneanet)
	gmXref, err := resoudre(g, mentionDePersonne(grp.GrandMere))
	if err != nil {
		return err
	}
	citerSiExiste(g, gmXref, sourceGeneanet)

	fam, err := trouverOuCreerFamille(g, gpXref, gmXref)
	if err != nil {
		return err
	}

	annee, ok := grp.GrandPere.MariageAnnee, grp.GrandPere.OkMariage
	if !ok {
		annee, ok = grp.GrandMere.MariageAnnee, grp.GrandMere.OkMariage
	}
	if ok && fam.Evenement("MARR") == nil {
		fam.AjouterLignes([]string{"1 MARR", "2 DATE " + strconv.Itoa(annee)})
	}

	if parentDuSujet.Nom != "" {
		if pXref, err := resoudre(g, mentionDePersonne(parentDuSujet)); err == nil {
			cablerEnfant(g, fam, pXref)
		}
	}

	for _, enfant := range grp.Enfants {
		xref, err := resoudre(g, mentionDePersonne(enfant))
		if err != nil {
			return err
		}
		cablerEnfant(g, fam, xref)
		citerSiExiste(g, xref, sourceGeneanet)
		marquerSansDescendance(g, xref, enfant.SansDescendance)
		if enfant.OkMariage {
			assurerMariageConnu(g, xref, enfant.MariageAnnee)
		}
	}
	return nil
}

// marquerSansDescendance ajoute "1 NCHI 0" (idempotent) — Geneanet indique qu'aucun
// enfant n'est répertorié pour cette personne ("✂").
func marquerSansDescendance(g *gedcom.Gedcom, xref string, sansDescendance bool) {
	if !sansDescendance {
		return
	}
	if r, ok := g.Get(xref); ok && r.Valeur("NCHI") == "" {
		r.AjouterLigne("1 NCHI 0")
	}
}

// assurerMariageConnu crée une FAM où seul xref est un conjoint connu (HUSB ou WIFE
// selon SEX, HUSB par défaut si inconnu), MARR/DATE=annee — sauf si xref a déjà une
// FAMS avec un MARR daté de cette même année (union déjà connue plus précisément
// ailleurs, ex. via une Union propre à sa fiche : ne pas dupliquer).
func assurerMariageConnu(g *gedcom.Gedcom, xref string, annee int) {
	r, ok := g.Get(xref)
	if !ok {
		return
	}
	for _, famXref := range r.Valeurs("FAMS") {
		fam, ok := g.Get(famXref)
		if !ok {
			continue
		}
		ev := fam.Evenement("MARR")
		if ev == nil {
			continue
		}
		if an, ok := gedcom.Annee(ev.Date()); ok && an == annee {
			return
		}
	}
	husb, wife := xref, ""
	if r.Valeur("SEX") == "F" {
		husb, wife = "", xref
	}
	fam, err := g.AddFamily(g.ProchainXref("F"), husb, wife, nil, "")
	if err != nil {
		return
	}
	r.AddFams(fam.Xref)
	fam.AjouterLignes([]string{"1 MARR", "2 DATE " + strconv.Itoa(annee)})
}

// cablerSources cite chaque puce de la fiche sur son sujet, dédupliquée globalement
// par texte. Raffinement best-effort : un label "Naissance"/"Décès" cite aussi
// l'événement correspondant — un label "Union"/"Mariage" n'essaie pas de retrouver la
// FAM précise (pas requis pour la correction, voir le plan) et retombe sur le défaut.
func cablerSources(g *gedcom.Gedcom, f *Fiche, sujet *gedcom.Record, sourcesXref map[string]string) {
	for _, s := range f.Sources {
		texte := s.Texte
		if texte == "" {
			texte = s.Label
		}
		if texte == "" {
			continue
		}
		xref, ok := sourcesXref[texte]
		if !ok {
			rec, err := g.AddSource(g.ProchainXref("S"), texte, "", "", "", "")
			if err != nil {
				continue
			}
			xref = rec.Xref
			sourcesXref[texte] = xref
		}
		sujet.AddCitation(xref, "")

		label := strings.ToLower(strings.TrimSpace(s.Label))
		switch {
		case strings.HasPrefix(label, "naissance"):
			sujet.AddCitation(xref, "BIRT")
		case strings.HasPrefix(label, "deces"), strings.HasPrefix(label, "déc"):
			sujet.AddCitation(xref, "DEAT")
		}
	}
}

func citerSiExiste(g *gedcom.Gedcom, xref, sourceGeneanet string) {
	if r, ok := g.Get(xref); ok {
		r.AddCitation(sourceGeneanet, "")
	}
}

// trouverOuCreerFamille cherche une FAM portant exactement (husb, wife) et n'en crée
// une que si aucune ne correspond — mêmes rôles fixes que add.trouverOuCreerFamilleParent.
func trouverOuCreerFamille(g *gedcom.Gedcom, husb, wife string) (*gedcom.Record, error) {
	for _, fam := range g.Familles() {
		if fam.Valeur("HUSB") == husb && fam.Valeur("WIFE") == wife {
			return fam, nil
		}
	}
	fam, err := g.AddFamily(g.ProchainXref("F"), husb, wife, nil, "")
	if err != nil {
		return nil, err
	}
	for _, x := range []string{husb, wife} {
		if x == "" {
			continue
		}
		if r, ok := g.Get(x); ok {
			r.AddFams(fam.Xref)
		}
	}
	return fam, nil
}

// trouverOuCreerFamilleConjoint cherche une FAM unissant déjà (a, b), dans n'importe
// quel ordre HUSB/WIFE, avant d'en créer une — nécessaire ici (contrairement à
// trouverOuCreerFamille) car la même union est souvent décrite depuis les deux
// fiches conjointes, chacune avec son propre "sujet" en premier argument.
func trouverOuCreerFamilleConjoint(g *gedcom.Gedcom, a, b, sexeA string) (*gedcom.Record, error) {
	for _, fam := range g.Familles() {
		h, w := fam.Valeur("HUSB"), fam.Valeur("WIFE")
		if (h == a && w == b) || (h == b && w == a) {
			return fam, nil
		}
	}
	husb, wife := rolesConjoint(g, a, b, sexeA)
	return trouverOuCreerFamille(g, husb, wife)
}

func rolesConjoint(g *gedcom.Gedcom, a, b, sexeA string) (husb, wife string) {
	switch {
	case sexeA == "F":
		return b, a
	case sexeA == "M":
		return a, b
	default:
		if r, ok := g.Get(b); ok && r.Valeur("SEX") == "M" {
			return b, a
		}
		return a, b
	}
}

func ajouterMariageSiAbsent(fam *gedcom.Record, u Union) {
	if fam.Evenement("MARR") != nil {
		return
	}
	if u.Mariage.Date == "" && u.Mariage.Lieu == "" {
		return
	}
	lignes := []string{"1 MARR"}
	if u.Mariage.Date != "" {
		lignes = append(lignes, "2 DATE "+u.Mariage.Date)
	}
	if u.Mariage.Lieu != "" {
		lignes = append(lignes, "2 PLAC "+u.Mariage.Lieu)
	}
	fam.AjouterLignes(lignes)
}

// cablerEnfant ajoute enfantXref en CHIL de fam (idempotent : la même union peut être
// décrite depuis plusieurs fiches) et le FAMC réciproque côté enfant.
func cablerEnfant(g *gedcom.Gedcom, fam *gedcom.Record, enfantXref string) {
	deja := false
	for _, x := range fam.Valeurs("CHIL") {
		if x == enfantXref {
			deja = true
			break
		}
	}
	if !deja {
		fam.AjouterLigne("1 CHIL @" + enfantXref + "@")
	}
	if r, ok := g.Get(enfantXref); ok {
		r.AddFamc(fam.Xref)
	}
}

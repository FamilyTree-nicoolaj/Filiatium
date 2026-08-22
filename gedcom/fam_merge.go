package gedcom

import (
	"fmt"
	"strings"
)

// blocsNiveau1 découpe lignes en unités "ligne de niveau 1 + ses sous-lignes" — un
// fait GEDCOM ("1 MARR" + "2 DATE" + "2 PLAC"...) est une unité de comparaison, jamais
// une ligne isolée. La ligne d'en-tête de niveau 0 (Lignes[0] d'un Record) ne démarre
// aucun bloc et disparaît silencieusement (voir merge.blocs, même logique).
func blocsNiveau1(lignes []string) [][]string {
	var out [][]string
	for _, l := range lignes {
		if d, ok := Decoupe(l); ok && d.Niveau == 1 {
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

// FusionnerFamilles fusionne fam2 dans fam1 au sein d'un même arbre : fam1 récupère le
// HUSB/WIFE que fam2 connaît et qu'elle ignore, les CHIL de fam2 qu'elle n'a pas déjà,
// et tout autre bloc de niveau 1 (MARR, NOTE, SOUR...) qu'elle n'a pas déjà à
// l'identique. Tout pointeur "@fam2@" du fichier (FAMC/FAMS des individus concernés,
// y compris un conjoint commun aux deux familles) est repointé vers fam1, les doublons
// de pointeur que ce repointage peut faire apparaître sur un individu sont nettoyés,
// puis fam2 est supprimée. Erreur (rien n'est modifié) si HUSB ou WIFE est connu des
// deux côtés mais différent — voir le commentaire plus bas, jamais tranché ici.
//
// N'effectue par ailleurs aucune vérification de similarité : c'est à l'appelant de
// garantir que la paire est un doublon confirmé avant d'appeler ceci (voir rules.D1,
// utilisé par `import --force D1`).
func (g *Gedcom) FusionnerFamilles(xref1, xref2 string) error {
	xref1, xref2 = strings.Trim(xref1, "@"), strings.Trim(xref2, "@")
	if xref1 == xref2 {
		return fmt.Errorf("fusion : @%s@ avec elle-même", xref1)
	}
	fam1, ok1 := g.Get(xref1)
	fam2, ok2 := g.Get(xref2)
	if !ok1 || fam1.Tag != "FAM" {
		return fmt.Errorf("fusion : @%s@ n'est pas une famille", xref1)
	}
	if !ok2 || fam2.Tag != "FAM" {
		return fmt.Errorf("fusion : @%s@ n'est pas une famille", xref2)
	}
	// Un HUSB/WIFE connu des deux côtés mais different n'est pas un doublon mécanique :
	// D1 les a rapprochées sur l'autre conjoint (voir rules.D1) et un enfant commun, mais
	// celui-ci pourrait être en fait la même personne que l'appariement d'individus n'a
	// pas reconnue (patronyme/OCR divergents) — fusionner sans discuter ferait disparaître
	// silencieusement son lien FAMS. Un jugement humain (voir `automerge`/`forcemerge`
	// pour fusionner ensuite les deux individus) reste nécessaire, jamais tranché ici.
	if h1, h2 := fam1.Valeur("HUSB"), fam2.Valeur("HUSB"); h1 != "" && h2 != "" && h1 != h2 {
		return fmt.Errorf("@%s@ et @%s@ : HUSB différent (@%s@ vs @%s@) — pas un doublon mécanique", xref1, xref2, h1, h2)
	}
	if w1, w2 := fam1.Valeur("WIFE"), fam2.Valeur("WIFE"); w1 != "" && w2 != "" && w1 != w2 {
		return fmt.Errorf("@%s@ et @%s@ : WIFE différent (@%s@ vs @%s@) — pas un doublon mécanique", xref1, xref2, w1, w2)
	}

	if fam1.Valeur("HUSB") == "" {
		if h := fam2.Valeur("HUSB"); h != "" {
			fam1.AjouterLigne("1 HUSB @" + h + "@")
		}
	}
	if fam1.Valeur("WIFE") == "" {
		if w := fam2.Valeur("WIFE"); w != "" {
			fam1.AjouterLigne("1 WIFE @" + w + "@")
		}
	}

	connus := map[string]bool{}
	for _, c := range fam1.Valeurs("CHIL") {
		connus[c] = true
	}
	for _, c := range fam2.Valeurs("CHIL") {
		if !connus[c] {
			fam1.AjouterLigne("1 CHIL @" + c + "@")
			connus[c] = true
		}
	}

	dejaPresent := map[string]bool{}
	for _, b := range blocsNiveau1(fam1.Lignes) {
		dejaPresent[strings.Join(b, "\n")] = true
	}
	repris := map[string]bool{"HUSB": true, "WIFE": true, "CHIL": true, "CHAN": true}
	for _, b := range blocsNiveau1(fam2.Lignes) {
		d, ok := Decoupe(b[0])
		if !ok || repris[d.Tag] {
			continue
		}
		if !dejaPresent[strings.Join(b, "\n")] {
			fam1.AjouterLignes(b)
		}
	}

	idx2 := -1
	for i, r := range g.Records {
		if r == fam2 {
			idx2 = i
			break
		}
	}
	g.Renumeroter(map[string]string{xref2: xref1})
	g.Records = append(g.Records[:idx2], g.Records[idx2+1:]...)

	ptrFams, ptrFamc := "1 FAMS @"+xref1+"@", "1 FAMC @"+xref1+"@"
	for _, ind := range g.Individus() {
		for ind.SupprimerOccurrenceEnTrop(ptrFams) {
		}
		for ind.SupprimerOccurrenceEnTrop(ptrFamc) {
		}
	}
	return nil
}

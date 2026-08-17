// Package gedcom lit et retouche un fichier GEDCOM 5.5.1 ligne par ligne — jamais en
// le reconstruisant depuis un modèle objet. On insère ou on remplace les lignes des
// enregistrements concernés ; tout le reste ressort identique à l'octet près. C'est
// ce qui permet de relire le `git diff` et de garantir qu'on n'a rien cassé ailleurs.
// Port de gedcom.py (~/Documents/Généalogie/outils/gedcom.py).
package gedcom

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ligneRe = regexp.MustCompile(`^(\d+) (?:@(\w+)@ )?([A-Za-z0-9_]+)(?: (.*))?$`)

// Ligne est une ligne GEDCOM décomposée : "2 DATE 5 JUN 1674" -> {2, "", "DATE", "5 JUN 1674"}.
type Ligne struct {
	Niveau int
	Xref   string
	Tag    string
	Valeur string
}

// Decoupe décompose une ligne GEDCOM brute. ok=false si la ligne est malformée.
func Decoupe(ligne string) (l Ligne, ok bool) {
	m := ligneRe.FindStringSubmatch(ligne)
	if m == nil {
		return Ligne{}, false
	}
	niveau, err := strconv.Atoi(m[1])
	if err != nil {
		return Ligne{}, false
	}
	return Ligne{Niveau: niveau, Xref: m[2], Tag: m[3], Valeur: m[4]}, true
}

// limiteConc est la coupe utilisée pour replier une ligne trop longue en CONC —
// mêmes 248 caractères que gedcom.py, laissant de la marge sous les 255 de GEDCOM 5.5.1.
const limiteConc = 248

// Enligne renvoie ["n TAG valeur"] avec des CONC de repli si la valeur dépasse 255
// caractères une fois assemblée. GEDCOM 5.5.1 limite chaque ligne à 255 caractères —
// des runes, pas des octets (voir README : family.ged a des lignes UTF-8 dont les
// accents portent plusieurs octets par caractère).
func Enligne(niveau int, tag, valeur string) []string {
	premiere := fmt.Sprintf("%d %s %s", niveau, tag, valeur)
	if utf8.RuneCountInString(premiere) <= 255 {
		return []string{premiere}
	}
	runes := []rune(valeur)
	lignes := []string{fmt.Sprintf("%d %s %s", niveau, tag, string(runes[:limiteConc]))}
	reste := runes[limiteConc:]
	for len(reste) > 0 {
		n := limiteConc
		if n > len(reste) {
			n = len(reste)
		}
		lignes = append(lignes, fmt.Sprintf("%d CONC %s", niveau+1, string(reste[:n])))
		reste = reste[n:]
	}
	return lignes
}

// EnligneNote découpe un texte multi-paragraphes en blocs NOTE/CONT/CONC. Un "\n"
// dans valeur sépare des paragraphes (-> CONT, qui insère un saut de ligne à la
// lecture) ; chaque paragraphe trop long pour 255 caractères est lui-même replié en
// CONC (voir Enligne). Le premier paragraphe porte NOTE, les suivants CONT.
func EnligneNote(niveau int, valeur string) []string {
	var lignes []string
	for i, paragraphe := range strings.Split(valeur, "\n") {
		tag := "CONT"
		if i == 0 {
			tag = "NOTE"
		}
		lignes = append(lignes, Enligne(niveau, tag, paragraphe)...)
	}
	return lignes
}

package gedcom

import (
	"regexp"
	"strconv"
)

var anneeRe = regexp.MustCompile(`\b(1[0-9]{3}|20[0-9]{2})\b`)

// Annee extrait la première année à quatre chiffres d'une date GEDCOM, ou ok=false.
// "BET 1700 AND 1710" -> 1700 ; "ABT 1676" -> 1676 ; "17 DEC 1710" -> 1710.
func Annee(dateGedcom string) (int, bool) {
	if dateGedcom == "" {
		return 0, false
	}
	m := anneeRe.FindString(dateGedcom)
	if m == "" {
		return 0, false
	}
	n, err := strconv.Atoi(m)
	if err != nil {
		return 0, false
	}
	return n, true
}

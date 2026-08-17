package gedcom

import "strings"

var accentsRepli = map[rune]rune{
	'à': 'a', 'â': 'a', 'ä': 'a', 'á': 'a',
	'ç': 'c',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'î': 'i', 'ï': 'i',
	'ô': 'o', 'ö': 'o',
	'ù': 'u', 'û': 'u', 'ü': 'u',
	'ÿ': 'y',
	'œ': 'o', // simplifié : "oe" décalerait la longueur, "o" suffit pour comparer
}

// Normaliser ramène une chaîne à une forme comparable, insensible à la casse et aux
// accents français courants : "Gélis" et "GELIS" normalisent tous deux vers "gelis".
// Table de repli manuelle plutôt que golang.org/x/text/unicode/norm : comparer des
// patronymes français est couvert par une quinzaine de runes, une dépendance externe
// serait disproportionnée. Utilisé par la recherche d'homonymes (add) et l'appariement
// de fusion (merge).
func Normaliser(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if repl, ok := accentsRepli[r]; ok {
			r = repl
		}
		b.WriteRune(r)
	}
	return b.String()
}

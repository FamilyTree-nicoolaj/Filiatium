package geneanet

import (
	"regexp"
	"strings"
)

var moisGedcom = map[string]string{
	"janvier": "JAN", "fevrier": "FEB", "février": "FEB", "mars": "MAR", "avril": "APR",
	"mai": "MAY", "juin": "JUN", "juillet": "JUL", "aout": "AUG", "août": "AUG",
	"septembre": "SEP", "octobre": "OCT", "novembre": "NOV", "decembre": "DEC", "décembre": "DEC",
}

var reDateFr = regexp.MustCompile(`(?i)^(\d{1,2})\s+([A-Za-zéèêëàâäôöùûüïîç]+)\s+(\d{3,4})$`)

// DateGedcom convertit une date en français ("3 septembre 1798") en date GEDCOM
// ("3 SEP 1798"). ok=false si le texte ne correspond pas au format attendu (bruit
// OCR, mois non reconnu) — le champ reste alors simplement absent, jamais deviné.
func DateGedcom(texteFr string) (string, bool) {
	m := reDateFr.FindStringSubmatch(strings.TrimSpace(texteFr))
	if m == nil {
		return "", false
	}
	mois, ok := moisGedcom[strings.ToLower(m[2])]
	if !ok {
		return "", false
	}
	return m[1] + " " + mois + " " + m[3], true
}

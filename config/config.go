// Package config regroupe les seuils de jugement réglables des règles de réalisme.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Seuils sont des seuils de jugement, pas des paramètres de comportement : ils se
// calibrent sur un corpus réel, d'où leur exposition (au lieu de constantes en dur).
type Seuils struct {
	AgeMinParent  int // âge minimal pour se marier / avoir un enfant (controle.py: AGE_MIN_PARENT)
	LongeviteMax  int // longévité maximale plausible (controle.py: LONGEVITE_MAX)
	EcartEpouxMax int // écart d'âge maximal plausible entre époux (controle.py: ECART_EPOUX_MAX)

	AgeMinMere           int // R7 : âge minimal plausible d'une mère
	AgeMaxMere           int // R7 : âge maximal plausible d'une mère
	AgeMaxPere           int // R8 : âge maximal plausible d'un père
	EcartGermainsMoisMin int // R9 : écart minimal plausible (en mois) entre deux naissances
}

// Defauts reprend telles quelles les constantes de controle.py, complétées par les
// seuils des règles de réalisme étendues (R7-R9).
func Defauts() Seuils {
	return Seuils{
		AgeMinParent:  13,
		LongeviteMax:  105,
		EcartEpouxMax: 40,

		AgeMinMere:           13,
		AgeMaxMere:           50,
		AgeMaxPere:           75,
		EcartGermainsMoisMin: 9,
	}
}

// Charger part de Defauts() et les surcharge avec un "filiatium.json" optionnel posé
// à côté du GEDCOM (mêmes noms de champs que Seuils, ex. {"AgeMaxMere": 55}). Absence
// de fichier n'est pas une erreur — c'est le cas normal.
func Charger(cheminGedcom string) (Seuils, error) {
	s := Defauts()
	chemin := filepath.Join(filepath.Dir(cheminGedcom), "filiatium.json")
	octets, err := os.ReadFile(chemin)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(octets, &s); err != nil {
		return s, fmt.Errorf("%s : %w", chemin, err)
	}
	return s, nil
}

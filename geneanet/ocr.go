package geneanet

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// TesseractDisponible vérifie que le binaire système tesseract est accessible avant
// de traiter la moindre image — un message d'installation actionnable plutôt qu'un
// échec opaque au premier fichier. tesseract est invoqué en interne par `import` :
// l'utilisateur ne le voit ni ne l'appelle lui-même.
func TesseractDisponible() error {
	if _, err := exec.LookPath("tesseract"); err != nil {
		return fmt.Errorf(`tesseract introuvable dans le PATH — installer via "brew install tesseract tesseract-lang" ` +
			`(ou "apt install tesseract-ocr tesseract-ocr-fra"), puis réessayer ; ou utiliser --texte ` +
			`si les fichiers sont déjà du texte OCR/copié-collé`)
	}
	return nil
}

// OCR extrait le texte d'une image via le binaire système tesseract : "-l fra"
// (français), "--psm 6" (bloc de texte uniforme — le seul réglage utile pour une
// capture de site web, sans bibliothèque de traitement d'image).
func OCR(cheminImage string) (string, error) {
	cmd := exec.Command("tesseract", cheminImage, "stdout", "-l", "fra", "--psm", "6")
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tesseract a échoué sur %s : %w (%s)", cheminImage, err, strings.TrimSpace(errs.String()))
	}
	return out.String(), nil
}

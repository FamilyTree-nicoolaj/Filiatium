package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// lancerInteractif est le point d'entrée quand filiatium est lancé sans argument
// (c'est ce que fait `make`). Il ne réimplémente rien : il pioche dans la même table
// `commandes` que l'aiguillage CLI, pour que les deux interfaces ne puissent jamais
// diverger — chaque commande ajoutée plus tard apparaît ici sans code supplémentaire.
func lancerInteractif() int {
	if !terminalInteractif() {
		// Une entrée non interactive (script, CI, sortie redirigée) ne doit jamais
		// rester bloquée à attendre une réponse qui ne viendra pas.
		afficherAide()
		return 2
	}
	lecteur := bufio.NewScanner(os.Stdin)

	chemin, ok := choisirFichier(lecteur)
	if !ok {
		return 2
	}

	fmt.Println("\nque faire ?")
	for i, c := range commandes {
		fmt.Printf("  %d. %s — %s\n", i+1, c.Nom, c.Description)
	}
	fmt.Print("> ")
	if !lecteur.Scan() {
		return 2
	}
	choix, err := strconv.Atoi(strings.TrimSpace(lecteur.Text()))
	if err != nil || choix < 1 || choix > len(commandes) {
		fmt.Fprintln(os.Stderr, "choix invalide")
		return 2
	}

	c := commandes[choix-1]
	fmt.Printf("\n→ filiatium %s %s\n\n", c.Nom, chemin)
	return c.Executer([]string{chemin})
}

func terminalInteractif() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func choisirFichier(lecteur *bufio.Scanner) (string, bool) {
	candidats, _ := filepath.Glob("*.ged")
	if len(candidats) == 0 {
		return demanderChemin(lecteur)
	}
	fmt.Println("fichiers GEDCOM trouvés :")
	for i, c := range candidats {
		fmt.Printf("  %d. %s\n", i+1, c)
	}
	fmt.Printf("  %d. (autre chemin)\n", len(candidats)+1)
	fmt.Print("> ")
	if !lecteur.Scan() {
		return "", false
	}
	choix, err := strconv.Atoi(strings.TrimSpace(lecteur.Text()))
	if err != nil || choix < 1 || choix > len(candidats)+1 {
		fmt.Fprintln(os.Stderr, "choix invalide")
		return "", false
	}
	if choix == len(candidats)+1 {
		return demanderChemin(lecteur)
	}
	return candidats[choix-1], true
}

func demanderChemin(lecteur *bufio.Scanner) (string, bool) {
	fmt.Print("chemin du fichier GEDCOM : ")
	if !lecteur.Scan() {
		return "", false
	}
	chemin := strings.TrimSpace(lecteur.Text())
	return chemin, chemin != ""
}

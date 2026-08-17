#!/usr/bin/env bash
# Compare, fichier par fichier, les signalements de `filiatium check` à ceux des
# scripts Python de référence (controle.py, controle_liens.py, controle_doublons.py).
# La correspondance recherchée est le TEXTE des signalements, pas seulement leur
# nombre : chaque ligne indentée produite par les deux chaînes doit être identique
# une fois triée. valider.py n'a pas d'équivalent ligne à ligne (S4/S5 regroupent
# différemment, voir gedcom/README) : seul le compte d'anomalies y est comparé.
#
# Le réalisme ne compare que R1-R6 (--regle, pas --categorie) : R7-R13 sont des
# règles étendues qui n'ont pas d'équivalent dans controle.py (A-F), les comparer à
# la catégorie complète ferait remonter un "écart" sur chaque extension légitime.
#
# usage : scripts/parite.sh <dossier-outils-python> <dossier-corpus>
set -euo pipefail
outils="${1:?usage: scripts/parite.sh <dossier-outils-python> <dossier-corpus>}"
corpus="${2:?usage: scripts/parite.sh <dossier-outils-python> <dossier-corpus>}"

cd "$(dirname "$0")/.."
go build -o filiatium .

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echec=0
compare() {
	local fichier="$1" selecteur_go="$2" script="$3" etiquette="$4"
	python3 "$outils/$script" "$fichier" | grep '^    ' | sort >"$tmp/py.txt" || true
	./filiatium check "$fichier" $selecteur_go | grep '^    ' | sort >"$tmp/go.txt" || true
	if ! diff -u "$tmp/py.txt" "$tmp/go.txt" >"$tmp/diff.txt"; then
		echo "ÉCART [$etiquette] $(basename "$fichier") :"
		cat "$tmp/diff.txt"
		echec=1
	fi
}

fichiers=("$corpus/family.ged")
while IFS= read -r -d '' f; do fichiers+=("$f"); done < <(find "$corpus/secondary_trees" -name '*.ged' -print0 2>/dev/null)

for f in "${fichiers[@]}"; do
	echo "=== $(basename "$f")"
	compare "$f" "--categorie liens" controle_liens.py liens
	compare "$f" "--categorie doublons" controle_doublons.py doublons
	compare "$f" "--regle R1,R2,R3,R4,R5,R6" controle.py realisme

	py_struct=$(python3 "$outils/valider.py" "$f" | grep -c '⚠' || true)
	go_struct=$(./filiatium check "$f" --categorie structure | tail -1 | grep -oE '[0-9]+' || true)
	if [ "$py_struct" != "$go_struct" ]; then
		echo "ÉCART [structure] $(basename "$f") : python=$py_struct go=$go_struct"
		echec=1
	fi
done

if [ "$echec" -eq 0 ]; then
	echo
	echo "✓ parité stricte sur $(( ${#fichiers[@]} )) fichier(s)"
else
	exit 1
fi

#!/usr/bin/env bash
# Vérifie que Load+Save ne change pas un octet sur le corpus généalogique réel.
# Le corpus n'est jamais copié dans ce dépôt (données personnelles) : ce script
# pointe la vérification vers un dossier externe. Voir gedcom/corpus_test.go.
set -euo pipefail
corpus="${1:?usage: scripts/roundtrip.sh <dossier-corpus>}"
FILIATIUM_CORPUS="$corpus" go test ./gedcom/... -run TestRoundTripCorpusExterne -v

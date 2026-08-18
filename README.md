# filiatium

Outil unique de validation et correction de GEDCOM 5.5.1 (compatibilité Gramps) :
vérifier, corriger ce qui est mécaniquement sûr, ajouter un individu en câblant tous
ses liens de parenté sans ambiguïté, appliquer des correctifs déclaratifs, analyser
si deux arbres sont fusionnables, et renuméroter proprement un GEDCOM.

Écrit en Go, zéro dépendance externe (stdlib seule), binaire statique.

**[Releases](https://github.com/FamilyTree-nicoolaj/filiatium/releases)** —
binaires précompilés (macOS universel, macOS arm64/amd64, Linux amd64/arm64,
Windows amd64), publiés automatiquement à chaque tag `vX.Y.Z`
(`.github/workflows/release.yml`).

## Installer / compiler

```bash
make          # compile puis lance le mode guidé
make build    # compile seulement -> ./filiatium
make install  # go install dans $GOBIN
```

Sans `make`, `go build .` suffit partout où Go est installé (macOS, Linux,
Windows). `make distribution` compile pour les cinq cibles principales
(`darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`, `windows/amd64`)
plus le binaire universel macOS, dans `dist/` — c'est ce que produit la release
GitHub ci-dessus.

## Commandes

Sans argument, `filiatium` lance un **mode guidé** (menu interactif) — pratique en
main, mais chaque action a un équivalent scriptable ci-dessous, affiché avant
exécution par le mode guidé lui-même.

### `check` — vérifier

```bash
filiatium check family.ged
filiatium check family.ged --categorie realisme
filiatium check family.ged --regle L1,L2,D1
filiatium check family.ged --avant family.ged.bak-2026-08-04
filiatium check family.ged --json
```

Toutes les règles du registre, groupées en 4 catégories (le compte exact et à
jour : `filiatium check help` ou `filiatium --ia`) :

| Règles | Catégorie | Contrôle |
|---|---|---|
| S1–S5 | `structure` | ligne malformée, saut de niveau, ligne > 255 caractères, pointeur non résolu, HEAD/TRLR |
| L1–L6 | `liens` | réciprocité FAMC/FAMS ↔ HUSB/WIFE/CHIL |
| D1–D4 | `doublons` | familles quasi-identiques, germains mariés, pointeurs répétés |
| R1–R6 | `realisme` | mariage recopié des parents, âges au mariage, longévité, dates identiques, écart d'âge des époux |
| R7–R13 | `realisme` | mère/père trop jeune ou âgé, germains trop rapprochés, mariage après décès, date future, ordre baptême/naissance et inhumation/décès, décès manquant probable |

Seuils de jugement (`AgeMinParent`, `LongeviteMax`, `EcartEpouxMax`, `AgeMinMere`,
`AgeMaxMere`, `AgeMaxPere`, `EcartGermainsMoisMin`) réglables via un
`filiatium.json` optionnel posé à côté du GEDCOM :

```json
{"AgeMaxMere": 55}
```

Code de sortie : `0` si rien à signaler, `1` sinon.

### `fix` — corriger ce qui est mécaniquement sûr

Trois corrections seulement, toutes non destructrices : lien réciproque manquant
(L1/L2), pointeur dupliqué (D3/D4), ligne > 255 caractères repliée en `CONC` (S3).

```bash
filiatium fix family.ged                # simulation (rien n'est écrit)
filiatium fix family.ged --write        # applique
filiatium fix family.ged --interactif   # confirme chaque correction (o/n/tout/aucun)
```

Après `--write`, tout le registre de règles est rejoué automatiquement ; l'écriture
est annulée si une correction introduit un signalement nouveau.

### `add` — ajouter un individu sans ambiguïté

```bash
filiatium add family.ged --nom "Jean /Dupret/" --sexe M \
  --naiss "12 MAR 1805" --pere I0123 --mere I0124 --conjoint I0200 --write
```

Recherche d'homonyme avant création (refuse par défaut si un candidat existe ;
`--force` passe outre). Câble systématiquement les **deux sens** de chaque lien
(`FAM.CHIL` + `INDI.FAMC` ; `FAM.HUSB`/`WIFE` + `INDI.FAMS`) — y compris côté
parent quand une nouvelle famille est créée. Rejoue le registre de règles avant
d'écrire ; refuse si l'ajout introduit un signalement nouveau.

Trois façons de fournir les données :
- options en ligne de commande (ci-dessus) ;
- `--fichier lot.json` : un objet ou un tableau d'objets aux mêmes champs
  (`Nom`, `Sexe`, `Naissance`, `Deces`, `Pere`, `Mere`, `Conjoint`, `Note`), pour des
  ajouts en lot ;
- assistant interactif si ni `--nom` ni `--fichier` ne sont fournis (terminal
  requis — voir « Usage par un script ou un agent » plus bas).

### `apply` — correctifs déclaratifs JSON

Remplace un script `patch_*.py` à usage unique par un fichier JSON versionnable,
auto-invalidant (une précondition qui ne tient plus refuse le rejeu) :

```json
{
  "cible": "family.ged",
  "justification": "Acte de mariage AD Tarn 5E123 vue 42",
  "preconditions": [{"xref": "F0111", "evenement": "MARR", "date_vaut": "5 JUN 1674"}],
  "operations": [
    {"op": "set_event_date", "xref": "F0111", "evenement": "MARR", "valeur": "27 MAY 1700"},
    {"op": "add_citation", "xref": "F0111", "source": "S0008", "evenement": "MARR"}
  ]
}
```

```bash
filiatium apply correctif.json           # simulation
filiatium apply correctif.json --write
```

`cible` est résolu relativement au dossier du fichier de correctif, pas au
répertoire courant. Opérations disponibles : `set_event_date`, `add_citation`,
`add_fams`, `add_famc`, `add_lines`, `add_source`, `add_individual`, `add_family`,
`add_record` (enregistrement entier, utilisé par `merge`), `set_line`,
`remove_line`, `touch_chan`.

`add_lines` insère des lignes neuves dans une fiche **déjà existante**, juste
avant son `1 CHAN` — le cas qui manquait entre « créer un enregistrement entier »
(`add_record`) et « remplacer/supprimer une ligne déjà présente »
(`set_line`/`remove_line`) : ajouter un `BIRT`/`OCCU`/`NOTE` à quelqu'un qui n'en
avait pas du tout.

```json
{"op": "add_lines", "xref": "I0042", "lignes": ["1 BIRT", "2 DATE 12 MAR 1805"]}
```

### `merge --analyse` — deux GEDCOM sont-ils fusionnables ?

N'écrit jamais de GEDCOM. Identifie les enregistrements par leur **contenu**,
jamais par leurs xref (qui peuvent coïncider par accident — deux exports d'une
même base Gramps — ou diverger totalement). Produit un rapport (collisions de
xref, appariements d'individus classés *certaine*/*probable*/*à examiner* avec
leurs critères et conflits, contradictions qu'introduirait la fusion) et,
optionnellement, un plan de fusion au format `apply` :

```bash
filiatium merge --analyse family.ged secondary_trees/sicard-binas-1779.ged
filiatium merge --analyse base.ged apport.ged --plan fusion.json --fusionner certaines
filiatium apply fusion.json --write   # après relecture du rapport
```

Le plan **réutilise tel quel** ce qui est déjà identique dans `base`, **complète**
les fiches appariées avec les lignes qui leur manquent (ex. une famille dont un
export a gardé les enfants et l'autre les parents), et n'**insère** vraiment de
nouveaux enregistrements que pour ce qui reste — en renumérotant seulement en cas
de collision réelle de xref.

`--fusionner` règle jusqu'où le plan fusionne automatiquement, chaque niveau
incluant le précédent : `identiques` (uniquement le contenu octet-identique, aucun
jugement) < `certaines` (+ appariements certains, individus et familles qui en
découlent — **défaut**) < `probables` (+ score 40-69) < `tout` (+ *à examiner*).
Un appariement au-delà du niveau choisi, ou un bloc en conflit de valeur (ex. deux
dates de mariage différentes), reste visible au rapport mais n'entre jamais dans
le plan : dédupliquer du contenu identique ou compléter une fiche avec ce qui lui
manque n'est pas un arbitrage (rien n'est choisi ni perdu) ; fusionner deux fiches
qui se *ressemblent* au-delà de `certaines`, ou trancher une valeur qui diverge,
reste un jugement humain, à faire à la lecture du rapport.

### `renumber` — renumérotation complète

Renumérote tous les xref INDI/FAM d'un GEDCOM (jamais SOUR/NOTE/OBJE/SUBM/REPO,
qui gardent les leurs), selon une des trois stratégies suivantes :

```bash
filiatium renumber family.ged --source I0001 --table renum.json  # simulation
filiatium renumber family.ged --source I0001 --table renum.json --write
filiatium renumber secondary.ged --decalage 5000 --write          # I0001 -> I5001
filiatium renumber secondary.ged --prefixe Z --write               # I0001 -> ZI0001
```

- `--source <xref>` : **numérotation cohérente**, en repartant de l'individu
  choisi — parcours en largeur (conjoints, enfants, parents, toutes pédigrées
  FAMC) ; toute composante déconnectée est ensuite balayée à son tour, en ordre
  fichier, pour que chaque INDI/FAM change bien de xref.
- `--decalage <n>` / `--prefixe <lettre>` : fonctions pures par-enregistrement
  (pas de parcours), pour **namespacer un arbre secondaire** avant de
  l'analyser avec `merge --analyse`, plutôt que de laisser `merge` renumérer
  au cas par cas sur collision réelle.

`--table <fichier>` écrit la correspondance ancien→nouveau xref en JSON,
indépendamment de `--write` (comme `--plan` sur `merge`). Une renumérotation
est une relabelisation bijective pure : elle ne change jamais ce que `check`
signale, seuls les xref cités dans les messages changent.

Mise à jour des notes de recherche, en étape **séparée et explicite** — jamais
embarquée dans le `--write` qui renumérote le `.ged` :

```bash
filiatium renumber --depuis-table renum.json                    # simulation, dossier = celui du .ged
filiatium renumber --depuis-table renum.json --notes ~/Documents/Genealogie --write
```

Rejoue la table sur les `*.md` du dossier (motif non récursif) : chaque
occurrence en **mot entier** d'un ancien xref (`I0517`, `[I0517]`, ou cité
`@F0271@` dans une ligne GEDCOM reproduite) est remplacée par le nouveau —
jamais un motif générique, seulement les xref réellement issus de cette
renumérotation précise.

## Usage par un script ou un agent

```bash
filiatium --ia
```

Affiche, en JSON sur stdout, un manifeste complet de l'outil : chaque commande
avec ses arguments positionnels, toutes ses options (nom, type, valeur par
défaut, description — dérivées par introspection des vraies définitions de
drapeaux, jamais recopiées à la main), le registre complet des règles de
`check`, les codes de sortie et quelques conseils d'usage. De quoi découvrir toute la
surface pilotable de l'outil sans parser la sortie texte de `--help`.

Toutes les commandes acceptent aussi `--json` (sortie structurée sur stdout,
erreurs sur stderr) et ont des codes de sortie stables : `0` rien à signaler /
succès, `1` signalements présents ou écriture refusée (auto-vérification),
`2` erreur d'usage ou d'E/S.

Aucune commande ne lit l'entrée standard **si les options nécessaires sont
fournies** : `check`, `fix` (sans `--interactif`), `add` (avec `--nom` ou
`--fichier`), `apply`, `merge`, `renumber` sont entièrement pilotables par
arguments, sans
jamais attendre de saisie. Seuls trois cas lisent stdin : `filiatium` sans
argument (menu guidé), `add` sans `--nom`/`--fichier` (assistant), et
`fix --interactif` — à réserver à un usage humain en terminal ; un script ou un
agent doit toujours passer les options requises plutôt que de compter sur la
détection de terminal.

## Développement

```bash
make test        # tous les tests
make verif       # format + vet + tests, à passer avant tout commit
make parite      # compare les signalements Go aux 4 scripts Python de référence
make roundtrip   # vérifie que charger+réécrire ne change pas un octet
make fusion      # analyse chaque arbre secondaire face à l'arbre principal
```

`parite`, `roundtrip` et `fusion` pointent par défaut vers
`~/Documents/Généalogie` (`make parite CORPUS=/autre/chemin` pour changer) — ce
corpus contient des données personnelles et n'est jamais copié dans ce dépôt.

## Structure du dépôt

```
gedcom/   bibliothèque de lecture/retouche GEDCOM (remplace gedcom.py)
rules/    registre de règles S1-S5, L1-L6, D1-D4, R1-R13
config/   seuils réglables (config.Defauts / config.Charger)
fix/      détection + application des 3 corrections mécaniques
add/      ajout d'individu auto-vérifié
patch/    correctifs déclaratifs JSON (préconditions + opérations)
merge/    analyse de fusion (identité par contenu, appariement, plan dédupliquant)
renumber/ renumérotation complète des xref INDI/FAM (source, décalage, préfixe)
cmd_*.go  sous-commandes CLI ; main.go/interactif.go l'aiguillage et le menu guidé
scripts/  parite.sh, roundtrip.sh — recette de non-régression contre le corpus réel
```

## Hors périmètre (délibérément)

GEDCOM 7, ANSEL, suppression automatique des pointeurs morts, interface graphique
— voir le plan de conception pour le raisonnement détaillé de chaque exclusion.
`merge` déduplique automatiquement le contenu octet-identique et complète les
fiches appariées avec certitude (aucune information choisie ni perdue), mais ne
fusionne jamais deux fiches qui se *ressemblent* seulement, ni ne tranche un bloc
en conflit de valeur : ça reste un jugement humain.

## Licence

MIT — voir [LICENSE](LICENSE). Nicolas Jalibert.
https://github.com/FamilyTree-nicoolaj/filiatium

Historique des versions : [CHANGELOG.md](CHANGELOG.md).

# Journal des versions

Format inspiré de [Keep a Changelog](https://keepachangelog.com/fr/), versionnage
[SemVer](https://semver.org/lang/fr/).

## [1.4.0] — 2026-08-18

### Ajouté

- Nouvelle commande `renumber` : renumérotation complète des xref INDI/FAM
  d'un GEDCOM (jamais SOUR/NOTE/OBJE/SUBM/REPO, qui gardent les leurs), selon
  trois stratégies au choix — `--source <xref>` (numérotation cohérente par
  parcours en largeur depuis un individu racine, avec balayage des composantes
  déconnectées), `--decalage <n>` (décale tous les numéros, ex. `+5000`), ou
  `--prefixe <lettre>` (ajoute une lettre devant chaque xref existant, ex.
  `I0001` → `ZI0001`) — utile pour namespacer un arbre secondaire avant de
  l'analyser avec `merge --analyse`.
- `renumber --table` écrit la correspondance ancien→nouveau xref en JSON ;
  `renumber --depuis-table` la rejoue ensuite, en étape séparée et explicite,
  sur les fichiers `.md` de recherche qui citent ces xref en texte libre
  (`--notes <dossier>`, défaut : celui du `.ged`).

## [1.3.0] — 2026-08-18

### Modifié

- `merge` : le plan de fusion (`--plan`) ne concatène plus systématiquement
  l'apport en préfixant tous ses xref. Il identifie désormais les enregistrements
  par leur **contenu** (jamais par leurs xref, qui peuvent coïncider par accident
  entre deux exports d'une même base Gramps, ou diverger totalement), réutilise
  tel quel ce qui est déjà identique dans la base, **complète** les fiches
  appariées avec les lignes qui leur manquent (ex. une famille dont un export a
  gardé les enfants et l'autre les parents), et ne renumérote plus qu'en cas de
  collision réelle de xref.
- `merge` apparie désormais aussi les **familles** (via leur couple/enfants déjà
  appariés), pas seulement les individus.
- Le score d'appariement des individus retranche désormais des points pour
  chaque conflit de fait (prénom, naissance, sexe) au lieu de se contenter de les
  lister sans effet sur le score — un rapprochement contredit sur trois points à
  la fois (ex. même patronyme, prénom/naissance/sexe tous différents) n'est plus
  affiché du tout, alors qu'il l'était même sans le moindre point de score
  positif hors le patronyme.
- Nouvelle option `--fusionner identiques|certaines|probables|tout` (défaut
  `certaines`) : règle jusqu'où le plan fusionne automatiquement. Chaque niveau
  inclut le précédent ; au-delà, un rapprochement reste visible au rapport mais
  n'entre jamais dans le plan.
- `--prefixe` disparaît (le préfixe de renumérotation est désormais dérivé
  automatiquement du xref d'origine, et ne sert plus qu'aux collisions réelles).
- Format JSON de `merge --analyse` : le champ `prefixe_suggere` disparaît au
  profit de `niveau`, `identiques`, `completees`, `nouveaux`, `renumerotes` et
  `conflits_non_appliques`.

## [1.2.0] — 2026-08-17

### Ajouté

- `apply` : nouvelle opération `add_lines`, pour insérer des lignes neuves dans
  une fiche **existante** — le cas qui manquait entre « créer un enregistrement
  entier » (`add_record`) et « remplacer/supprimer une ligne déjà présente »
  (`set_line`/`remove_line`). Sert notamment à ajouter un `BIRT`/`OCCU`/`NOTE` à
  quelqu'un qui n'en avait pas du tout, cas fréquent dans le flux de correctifs.
  Réutilise `Record.AjouterLigne`, déjà exposée côté bibliothèque mais jamais
  branchée sur `apply` jusqu'ici.

## [1.1.0] — 2026-08-17

### Ajouté

- Aide complète par sous-commande : `filiatium <commande> help` (ainsi que
  `--help`/`-h`) affiche désormais description, toutes les options et des
  exemples — pour `check`, en plus le tableau complet des règles, généré depuis
  `rules.Registre` (jamais recopié en dur, ne peut donc pas diverger).
  Nécessaire pour un binaire distribué seul (releases GitHub), sans dépôt ni
  README à côté : il doit pouvoir se documenter entièrement lui-même.
  `filiatium help` (sans sous-commande) reste le résumé succinct existant.
- `filiatium --about` et `filiatium help` rappellent désormais ce point d'entrée.
- `filiatium --ia` : manifeste JSON complet (commandes, options avec type/défaut/
  description, positionnels, registre des règles, codes de sortie, conseils
  d'usage), pour qu'un agent découvre toute la surface pilotable de l'outil sans
  parser du texte destiné à un humain. Les options sont dérivées par
  introspection des vrais `*flag.FlagSet` (chaque commande factorise son
  enregistrement de drapeaux dans une fonction `flagsXxx` réutilisée à la fois
  pour l'exécution et pour le manifeste) — jamais recopiées à la main, donc
  jamais susceptibles de diverger des options qui existent vraiment.
- `--ia` décrit aussi, pour `add`, le format JSON accepté par `--fichier`
  (`fichier_json`) : description, forme (un objet pour un individu, un tableau
  pour en ajouter plusieurs d'un coup) et la liste des champs, énumérée par
  réflexion sur `add.Requete` — seule la prose de chaque description reste
  écrite à la main.

### Modifié

- Plus aucun nombre de règles écrit en dur (l'ancien texte disait « 19 »,
  la vraie valeur est 28) : README et `filiatium check help` l'affichent
  désormais via `len(rules.Registre)`, la même source que `--ia`.

## [1.0.0] — 2026-08-17

Première version : portage complet et vérifié des quatre scripts Python de
`~/Documents/Généalogie/outils/` (`gedcom.py`, `valider.py`, `controle.py`,
`controle_liens.py`, `controle_doublons.py`) en un outil unique, plus les
fonctionnalités que l'ancien outillage n'avait pas.

### Ajouté

- **`gedcom`** — bibliothèque de lecture/retouche GEDCOM 5.5.1 ligne par ligne
  (remplace `gedcom.py`) : round-trip octet-pour-octet, garde de concurrence
  (SHA-256) à l'écriture, préservation BOM/CRLF.
- **`check`** — S1–S5 (structure, port de `valider.py`), L1–L6 (liens
  FAMC/FAMS, port de `controle_liens.py`), D1–D4 (doublons structurels, port de
  `controle_doublons.py`), R1–R6 (réalisme, port de `controle.py`) et **R7–R13**
  (réalisme étendu, nouveau : âge des parents, germains rapprochés, mariage après
  décès, dates futures, ordre des événements, décès manquant probable). Parité
  texte-exacte vérifiée avec les scripts Python sur les 17 fichiers du corpus réel.
- **`fix`** — corrections mécaniques non destructrices (lien réciproque manquant,
  pointeur dupliqué, ligne > 255 caractères), simulation par défaut, `--write`,
  `--interactif`, auto-vérification avant écriture.
- **`add`** — ajout d'individu non équivoque : recherche d'homonyme, câblage
  systématique des deux sens de chaque lien de parenté, auto-vérification par
  rejeu du registre de règles.
- **`apply`** — correctifs déclaratifs JSON (remplace le flux `patch_*.py`) :
  préconditions auto-invalidantes, opérations `set_event_date`, `add_citation`,
  `add_fams`, `add_famc`, `add_source`, `add_individual`, `add_family`,
  `add_record`, `set_line`, `remove_line`, `touch_chan`.
- **`merge --analyse`** — nouveau : analyse de fusion entre deux GEDCOM
  (collisions de xref, appariement d'individus scoré avec propagation par la
  parenté, contradictions structurelles introduites par une fusion mécanique) et
  génération d'un plan de fusion exécutable via `apply`.
- Mode guidé (`filiatium` sans argument) et `--about`/`--version`/`--help`.
- Seuils de réalisme réglables via `filiatium.json`.
- Sortie `--json` sur toutes les commandes, codes de sortie stables (0/1/2), pour
  un usage scripté ou par un agent.
- `Makefile` (`run`, `build`, `test`, `verif`, `parite`, `roundtrip`, `fusion`,
  `distribution`) et binaire universel macOS (`lipo`) dans `distribution`.

### Non repris (délibérément)

GEDCOM 7, ANSEL, suppression automatique des pointeurs morts, fusion automatique
de fiches identifiées comme doublons, interface graphique. Les 71 scripts
`patch_*.py`, les générateurs d'arbres secondaires et les scrapeurs d'archives
restent dans `~/Documents/Généalogie/outils/` (historique daté / hors périmètre).

### Connu

Les valeurs par défaut `GED`/`fichiers` du `Makefile` et de `scripts/parite.sh`
supposent un fichier nommé `family.ged` dans le corpus ; le fichier réel s'appelle
encore `jalibert2.ged`, d'où un `GED=...` explicite nécessaire pour `make fusion`
tant que l'un des deux noms n'est pas aligné.

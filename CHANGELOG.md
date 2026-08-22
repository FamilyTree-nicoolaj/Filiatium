# Journal des versions

Format inspiré de [Keep a Changelog](https://keepachangelog.com/fr/), versionnage
[SemVer](https://semver.org/lang/fr/).

## [2.2.2] — 2026-08-22

### Corrigé

- `import` : le glyphe ♂/♀ de tête OCRise en un jeton alphanumérique, mais pas au
  hasard pour une combinaison police/résolution donnée — confirmé sur données
  réelles, "Q" et "d'" (avec ou sans espace après l'apostrophe) sont respectivement
  le rendu dominant de ♀ et ♂, désormais reconnus comme signal de sexe à part
  entière (au lieu de rester attachés au prénom comme bruit). "os" (rendu observé
  pour un marqueur d'union, pas le sexe) est retiré sans valeur de sexe assignée.
- Le marqueur de décès seul "†<année>" OCRise parfois en "+<année>" (ex. "Charles
  SOIL +1805") — non reconnu jusqu'ici, l'année entière était alors prise à tort
  pour un patronyme ("Charles SOIL /+1805/"). "+" est maintenant toléré au même
  titre que "†".

## [2.2.1] — 2026-08-22

### Corrigé

- `import` : le parseur de fiches Geneanet supposait une puce `"- "` fixe et un
  intitulé de section OCRisé sans bruit — faux sur de l'OCR tesseract réel (bruit de
  puce variable "+"/"°"/"="/"©"/"»"/"." selon la capture, intitulés parfois tronqués
  au point de ne plus être reconnaissables, ex. "Sources" → "SOUS"). Corrigeait à
  tort des fiches réelles : une puce "Sources" non reconnue laissait la section
  précédente ouverte, et ses lignes ("Label: texte") étaient câblées comme de faux
  enfants/conjoints. Refonte : la distinction "nouvelle entrée" vs "enfant imbriqué"
  (Union(s), Demi-frères) se fait désormais par le CONTENU de la ligne, jamais par sa
  puce ; les intitulés de section ne sont plus ancrés en fin de ligne ; un bloc
  "NOTES" (jusqu'ici non reconnu) est maintenant reconnu et ignoré ; une puce
  contenant ":" ne peut plus fabriquer un individu dans une liste plate de personnes
  (Parents/Frères et sœurs/Grands-parents).
- La déduplication inter-fiches échouait quand le glyphe ♂/♀ OCRisait différemment
  selon la mention de la même personne (ex. "9 Marie Françoise TILMONT" sur sa
  propre fiche vs "Q Marie Françoise TILMONT" en s'auto-listant dans sa fratrie) :
  un mot de tête ≤2 lettres est maintenant toléré des deux côtés de la comparaison
  de prénom (`geneanet.prenomsCompatibles`).

## [2.2.0] — 2026-08-22

### Ajouté

- Nouvelle commande `import` : construit un GEDCOM complet à partir de captures
  d'écran de fiches individuelles Geneanet (parents, union(s)/enfants, frères
  et sœurs, demi-frères et demi-sœurs, profession, sources, grands-parents
  paternels/maternels et leurs oncles/tantes), vers un nouveau fichier
  `dst.ged`. Sur le bloc grands-parents, le parent du sujet est câblé comme
  enfant du couple même quand Geneanet l'omet de la liste (déjà montré via
  "Parents") ; `✂` (sans descendance connue) devient `1 NCHI 0` ; `⚭`/`⊖
  (année)` (mariage sans détail) devient un `MARR`/`DATE`, sans dupliquer une
  union déjà connue plus précisément ailleurs. L'OCR est fait en interne via
  le binaire système
  `tesseract` (`-l fra`, `--psm 6`) — jamais invoqué à la main ; `--texte` évite
  cet appel quand les fichiers sont déjà du texte. Les personnes qui se
  recoupent entre plusieurs fiches sont automatiquement dédupliquées par
  patronyme et prénom normalisés et — quand elle est connue des deux côtés —
  la **même** année de naissance (jamais une fenêtre de tolérance, pour ne pas
  fusionner à tort deux germains homonymes nés à un an d'écart). `--auteur`
  attribue une source Geneanet partagée à l'utilisatrice/au contributeur
  source de la capture. Contrairement aux autres commandes d'écriture, la
  garde avant/après ignore la catégorie réalisme (attendue et légitime sur un
  arbre neuf comparé à un fichier vide) mais bloque toujours sur
  structure/liens/doublons. Dépendance d'exécution nouvelle (hors Go) :
  `tesseract` — voir `Formula/filiatium.rb` (`depends_on "tesseract"`,
  `"tesseract-lang"` pour le français).
- `gedcom.Nouveau()` : squelette `HEAD`/`TRLR` minimal en mémoire, pour bâtir
  un arbre entièrement neuf sans fichier existant à charger — utilisé par
  `import`.

## [2.1.0] — 2026-08-18

### Ajouté

- Nouvelle commande `publish` : retire d'un GEDCOM les faits datés (`BIRT`,
  `MARR`, `DEAT`...) qu'une règle de réalisme (R1-R13) juge invraisemblables
  et qu'aucune citation `SOUR` ne vient étayer, vers un nouveau fichier
  `dst.ged` — `src.ged` n'est jamais modifié. Un fait sourcé (`SOUR`
  n'importe où sur l'individu/la famille concerné) n'est jamais supprimé,
  même signalé. `--niveau strict|modere|large` (défaut `strict`) règle
  jusqu'où le doute profite au fait : `strict` ne retient que les
  impossibilités sans seuil réglable (R10/R11/R12), `modere` ajoute les
  coïncidences suspectes (R1/R5), `large` ajoute les 8 règles à seuil
  réglable restantes. `--interactif` confirme chaque suppression
  individuellement ; sans lui, toutes les suppressions du niveau choisi sont
  automatiques (mode automatique).
- `rules.Finding` gagne un champ `Faits` (événement précis mis en cause par
  chaque règle de réalisme) — sert à `publish` à cibler précisément quoi
  supprimer, sans reparser le texte du message.
- `gedcom.Record` gagne `ASource()` (une citation SOUR existe-t-elle n'importe
  où sur la fiche ?) et `SupprimerEvenement(tag)` (retire un bloc d'événement
  entier).

## [2.0.0] — 2026-08-18

### Cassé

- `merge` est renommé `automerge` — comportement inchangé (contenu/score,
  n'écrit jamais de GEDCOM), seul le nom de la commande change. Tout script
  invoquant `filiatium merge --analyse ...` doit être mis à jour.

### Ajouté

- Nouvelle commande `forcemerge` : fusionne directement deux GEDCOM dans un
  nouveau fichier `dst.ged` (jamais l'un des deux fichiers source, qui restent
  intacts) à partir de paires d'individus déclarées explicitement en argument
  (« mode miroir », ex. `I1001:I4001`) — ces ancres ne sont jamais remises en
  cause, mais l'appariement automatique (contenu, score, parenté) continue de
  tourner autour d'elles, au niveau choisi par `--fusionner`. Un fait qui
  diverge entre les deux sources garde la valeur de `srcA`, mais l'alternative
  de `srcB` est en plus préservée en `NOTE` sur la fiche concernée : sans
  étape humaine de relecture avant écriture, rien de ce qui existe dans l'une
  des deux sources ne disparaît silencieusement de `dst.ged`. Même garde que
  `fix`/`add`/`apply`/`automerge` : refuse d'écrire si la fusion introduit un
  signalement nouveau.
- Le moteur de `merge/` (interne, partagé par `automerge` et `forcemerge`)
  accepte désormais des appariements forcés qui priment sur l'appariement par
  contenu/score, et propagent aux enfants/conjoints exactement comme un
  appariement détecté automatiquement (`Appariement.Force`).

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

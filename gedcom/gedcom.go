package gedcom

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrConflitConcurrent signale que le fichier a changé sur disque depuis Load() :
// une autre session a probablement écrit entretemps. Save() la renvoie plutôt que
// d'écraser en silence. La réponse n'est jamais de forcer l'écriture : recharger
// avec Load() et rejouer le patch sur l'état frais — c'est la protection automatique
// d'un incident réel (une session a réécrit par-dessus le correctif d'une autre).
type ErrConflitConcurrent struct {
	Chemin string
}

func (e *ErrConflitConcurrent) Error() string {
	return fmt.Sprintf("%s a changé depuis le chargement : une autre session a probablement "+
		"écrit entretemps. Recharger et rejouer le patch sur l'état frais — ne jamais forcer "+
		"l'écriture par-dessus.", e.Chemin)
}

// Record est un enregistrement de niveau 0 : Lignes[0] est la ligne "0 @XREF@ TAG".
type Record struct {
	Lignes []string
	Xref   string
	Tag    string
}

func nouveauRecord(lignes []string) *Record {
	r := &Record{Lignes: lignes}
	if len(lignes) > 0 {
		if l, ok := Decoupe(lignes[0]); ok {
			r.Xref, r.Tag = l.Xref, l.Tag
		}
	}
	return r
}

// NewRecord construit un Record à partir de lignes déjà mises en forme (lignes[0]
// doit être "0 @XREF@ TAG"). Sert aux paquets qui doivent fabriquer des
// enregistrements bruts sans passer par un fichier (ex. merge, pour un enregistrement
// renuméroté).
func NewRecord(lignes []string) *Record { return nouveauRecord(lignes) }

// Valeurs renvoie toutes les valeurs des lignes "<niveau> TAG ..." (sans les @).
// niveau vaut 1 si omis — équivalent du paramètre par défaut de gedcom.py.
func (r *Record) Valeurs(tag string, niveau ...int) []string {
	n := 1
	if len(niveau) > 0 {
		n = niveau[0]
	}
	var out []string
	for _, ligne := range r.Lignes[1:] {
		if l, ok := Decoupe(ligne); ok && l.Niveau == n && l.Tag == tag {
			out = append(out, strings.Trim(l.Valeur, "@"))
		}
	}
	return out
}

// Valeur renvoie la première valeur de Valeurs(tag, niveau...), ou "" s'il n'y en a pas.
func (r *Record) Valeur(tag string, niveau ...int) string {
	v := r.Valeurs(tag, niveau...)
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

// Nom renvoie le NAME sans les barres obliques GEDCOM ("Jean /Dupret/" -> "Jean Dupret"),
// ou "?" si absent.
func (r *Record) Nom() string {
	n := r.Valeur("NAME")
	if n == "" {
		return "?"
	}
	return strings.TrimSpace(strings.ReplaceAll(n, "/", ""))
}

var patronymeRe = regexp.MustCompile(`/([^/]*)/`)

// PatronymeDeNom extrait le patronyme d'un NAME GEDCOM : "Jean /Dupret/" -> "Dupret".
// Fonctionne aussi sur un NAME qui n'a pas encore été inséré dans un Record (add en
// a besoin pour chercher des homonymes avant de créer l'individu).
func PatronymeDeNom(nom string) string {
	m := patronymeRe.FindStringSubmatch(nom)
	if m == nil {
		return ""
	}
	return m[1]
}

// PrenomDeNom extrait le(s) prénom(s) d'un NAME GEDCOM : "Jean /Dupret/" -> "Jean".
func PrenomDeNom(nom string) string {
	if i := strings.Index(nom, "/"); i >= 0 {
		return strings.TrimSpace(nom[:i])
	}
	return strings.TrimSpace(nom)
}

// Patronyme renvoie le nom de famille entre barres obliques du NAME, ou "" si absent.
func (r *Record) Patronyme() string { return PatronymeDeNom(r.Valeur("NAME")) }

// FamcPedi une entrée (famille, pedigree) par "1 FAMC" ; Pedi vaut "" s'il n'est pas
// précisé. Ne voit que les "1 FAMC" : le "2 FAMC" que porte un bloc "1 ADOP" désigne
// la même famille et ferait doublon.
type FamcPedi struct {
	Fam  string
	Pedi string
}

func (r *Record) FamcPedi() []FamcPedi {
	var out []FamcPedi
	for j := 0; j < len(r.Lignes); j++ {
		l, ok := Decoupe(r.Lignes[j])
		if !ok || l.Niveau != 1 || l.Tag != "FAMC" {
			continue
		}
		pedi := ""
		for k := j + 1; k < len(r.Lignes); k++ {
			lk, ok := Decoupe(r.Lignes[k])
			if !ok || lk.Niveau <= 1 {
				break
			}
			if lk.Tag == "PEDI" {
				pedi = lk.Valeur
			}
		}
		out = append(out, FamcPedi{Fam: strings.Trim(l.Valeur, "@"), Pedi: pedi})
	}
	return out
}

// Evenement est le bloc "1 TAG" et ses sous-lignes, repéré par ses bornes dans record.Lignes.
type Evenement struct {
	record     *Record
	debut, fin int
}

// Evenement renvoie le bloc de l'événement "1 TAG" (ex. Evenement("BIRT")), ou nil.
func (r *Record) Evenement(tag string) *Evenement {
	for j := 0; j < len(r.Lignes); j++ {
		l, ok := Decoupe(r.Lignes[j])
		if !ok || l.Niveau != 1 || l.Tag != tag {
			continue
		}
		k := j + 1
		for k < len(r.Lignes) {
			lk, ok := Decoupe(r.Lignes[k])
			if ok && lk.Niveau <= 1 {
				break
			}
			k++
		}
		return &Evenement{record: r, debut: j, fin: k}
	}
	return nil
}

func (e *Evenement) valeur(tag string) string {
	for _, ligne := range e.record.Lignes[e.debut+1 : e.fin] {
		if l, ok := Decoupe(ligne); ok && l.Niveau == 2 && l.Tag == tag {
			return l.Valeur
		}
	}
	return ""
}

func (e *Evenement) Date() string { return e.valeur("DATE") }
func (e *Evenement) Lieu() string { return e.valeur("PLAC") }

// Annee est le premier millésime de la date de l'événement, ou ok=false.
func (e *Evenement) Annee() (int, bool) { return Annee(e.Date()) }

// Date renvoie la DATE de l'événement tag, ou "" si l'événement ou sa date sont absents.
func (r *Record) Date(tag string) string {
	ev := r.Evenement(tag)
	if ev == nil {
		return ""
	}
	return ev.Date()
}

func empreinte(octets []byte) string {
	h := sha256.Sum256(octets)
	return hex.EncodeToString(h[:])
}

// Gedcom est un fichier GEDCOM chargé en mémoire, prêt pour une retouche ligne à ligne.
type Gedcom struct {
	Records []*Record
	Chemin  string

	empreinteChargement string
	bom                 bool // le fichier source portait-il un BOM UTF-8 ? -> réécrit tel quel
	crlf                bool // le fichier source utilisait-il CRLF ? -> réécrit tel quel
}

var bomUTF8 = []byte{0xEF, 0xBB, 0xBF}

// Load charge un fichier GEDCOM. BOM UTF-8 et fin de ligne (CRLF ou LF) sont détectés
// et mémorisés pour que Save() les restitue à l'identique — family.ged n'en a pas,
// mais un arbre secondaire ou un futur export pourrait différer, et un Save qui
// normaliserait en silence produirait un diff de tout le fichier.
func Load(chemin string) (*Gedcom, error) {
	octets, err := os.ReadFile(chemin)
	if err != nil {
		return nil, err
	}
	bom := bytes.HasPrefix(octets, bomUTF8)
	corps := octets
	if bom {
		corps = corps[len(bomUTF8):]
	}
	crlf := bytes.Contains(corps, []byte("\r\n"))
	sep := "\n"
	if crlf {
		sep = "\r\n"
	}
	lignesBrutes := strings.Split(string(corps), sep)
	// La scission ajoute un élément vide final quand le fichier se termine par un saut
	// de ligne (cas normal) : il ne représente aucune ligne réelle, on l'ignore.
	if n := len(lignesBrutes); n > 0 && lignesBrutes[n-1] == "" {
		lignesBrutes = lignesBrutes[:n-1]
	}

	var records []*Record
	var courant *Record
	for _, ligne := range lignesBrutes {
		switch {
		case strings.HasPrefix(ligne, "0 "):
			courant = nouveauRecord([]string{ligne})
			records = append(records, courant)
		case courant == nil:
			return nil, fmt.Errorf("ligne hors enregistrement : %q", ligne)
		default:
			// Y compris les lignes vides éventuelles : contrairement à gedcom.py, qui les
			// élimine silencieusement, on les conserve pour que Save() reproduise le
			// fichier à l'octet près quel que soit son contenu.
			courant.Lignes = append(courant.Lignes, ligne)
		}
	}
	return &Gedcom{
		Records: records, Chemin: chemin,
		empreinteChargement: empreinte(octets), bom: bom, crlf: crlf,
	}, nil
}

// Save écrit le fichier — et refuse si quelqu'un d'autre a écrit entretemps.
// Avant d'écraser Chemin (ou chemin, si fourni et différent), revérifie que le
// contenu sur disque est encore celui lu par Load() : sinon renvoie
// ErrConflitConcurrent plutôt que d'effacer en silence ce qu'une autre session a
// écrit. Un save() vers un autre fichier n'a rien à comparer.
func (g *Gedcom) Save(chemin string) (string, error) {
	cible := chemin
	if cible == "" {
		cible = g.Chemin
	}
	if g.empreinteChargement != "" && cible == g.Chemin {
		if actuels, err := os.ReadFile(cible); err == nil {
			if empreinte(actuels) != g.empreinteChargement {
				return "", &ErrConflitConcurrent{Chemin: cible}
			}
		}
	}

	sep := "\n"
	if g.crlf {
		sep = "\r\n"
	}
	var lignes []string
	for _, r := range g.Records {
		lignes = append(lignes, r.Lignes...)
	}
	corps := []byte(strings.Join(lignes, sep) + sep)
	nouveauxOctets := corps
	if g.bom {
		nouveauxOctets = append(append([]byte{}, bomUTF8...), corps...)
	}
	if err := os.WriteFile(cible, nouveauxOctets, 0o644); err != nil {
		return "", err
	}
	g.empreinteChargement = empreinte(nouveauxOctets)
	return cible, nil
}

// Backup crée une copie de sûreté manuelle "<nom>.bak-AAAA-MM-JJ" — usage ponctuel
// seulement : le flux normal s'appuie sur le commit git précédent, pas sur ce
// fichier. N'écrase jamais une sauvegarde existante : deux appels le même jour
// donnent ".bak-AAAA-MM-JJ" puis ".bak-AAAA-MM-JJ-2", etc.
func (g *Gedcom) Backup(suffixe string) (string, error) {
	if suffixe == "" {
		suffixe = time.Now().Format("2006-01-02")
	}
	original, err := os.ReadFile(g.Chemin)
	if err != nil {
		return "", err
	}
	base := g.Chemin + ".bak-" + suffixe
	cible, n := base, 1
	for {
		existant, err := os.ReadFile(cible)
		if os.IsNotExist(err) {
			break
		}
		if err == nil && bytes.Equal(existant, original) {
			return cible, nil // déjà sauvegardé à l'identique
		}
		n++
		cible = fmt.Sprintf("%s-%d", base, n)
	}
	if err := os.WriteFile(cible, original, 0o644); err != nil {
		return "", err
	}
	return cible, nil
}

// ParXref indexe les enregistrements par xref. Recalculé à chaque appel plutôt que
// mis en cache — comme la propriété par_xref de gedcom.py — pour ne jamais risquer un
// index périmé après un ajout ; à 600 individus / 250 familles le coût est négligeable.
// ponytail : O(n) par appel, à indexer si le corpus dépasse quelques milliers d'enregistrements.
func (g *Gedcom) ParXref() map[string]*Record {
	m := make(map[string]*Record, len(g.Records))
	for _, r := range g.Records {
		if r.Xref != "" {
			m[r.Xref] = r
		}
	}
	return m
}

// Get renvoie l'enregistrement portant ce xref (avec ou sans @).
func (g *Gedcom) Get(xref string) (*Record, bool) {
	r, ok := g.ParXref()[strings.Trim(xref, "@")]
	return r, ok
}

// Contains indique si ce xref désigne un enregistrement existant.
func (g *Gedcom) Contains(xref string) bool {
	_, ok := g.Get(xref)
	return ok
}

// DeType renvoie tous les enregistrements de niveau 0 portant ce tag.
func (g *Gedcom) DeType(tag string) []*Record {
	var out []*Record
	for _, r := range g.Records {
		if r.Tag == tag {
			out = append(out, r)
		}
	}
	return out
}

func (g *Gedcom) Individus() []*Record { return g.DeType("INDI") }
func (g *Gedcom) Familles() []*Record  { return g.DeType("FAM") }

func ligneChanMaintenant() []string {
	maintenant := time.Now()
	return []string{
		"1 CHAN",
		"2 DATE " + strings.ToUpper(maintenant.Format("2 Jan 2006")),
		"3 TIME " + maintenant.Format("15:04:05"),
	}
}

func (g *Gedcom) inserer(r *Record, apres string) (*Record, error) {
	pos := -1
	if apres != "" {
		apres = strings.Trim(apres, "@")
		for j, rec := range g.Records {
			if rec.Xref == apres {
				pos = j + 1
				break
			}
		}
		if pos == -1 {
			return nil, fmt.Errorf("@%s@ n'existe pas", apres)
		}
	} else {
		for j, rec := range g.Records {
			if rec.Tag == "TRLR" {
				pos = j
				break
			}
		}
		if pos == -1 {
			pos = len(g.Records)
		}
	}
	out := make([]*Record, 0, len(g.Records)+1)
	out = append(out, g.Records[:pos]...)
	out = append(out, r)
	out = append(out, g.Records[pos:]...)
	g.Records = out
	return r, nil
}

// InsererRecord insère un Record déjà construit (voir NewRecord), après le xref
// `apres` ou juste avant TRLR si "". Primitive générique pour `merge`, dont le plan
// de fusion insère des enregistrements entiers plutôt que de les construire champ
// par champ comme AddIndividual/AddFamily/AddSource.
func (g *Gedcom) InsererRecord(r *Record, apres string) (*Record, error) {
	return g.inserer(r, apres)
}

// AddSource crée un enregistrement "0 @XREF@ SOUR". note peut être multi-paragraphes
// (voir EnligneNote). apres = xref après lequel insérer (sinon juste avant TRLR).
func (g *Gedcom) AddSource(xref, titl, auth, publ, note, apres string) (*Record, error) {
	xref = strings.Trim(xref, "@")
	if g.Contains(xref) {
		return nil, fmt.Errorf("@%s@ existe déjà", xref)
	}
	lignes := append([]string{"0 @" + xref + "@ SOUR"}, Enligne(1, "TITL", titl)...)
	if auth != "" {
		lignes = append(lignes, Enligne(1, "AUTH", auth)...)
	}
	if publ != "" {
		lignes = append(lignes, Enligne(1, "PUBL", publ)...)
	}
	if note != "" {
		lignes = append(lignes, EnligneNote(1, note)...)
	}
	lignes = append(lignes, ligneChanMaintenant()...)
	return g.inserer(nouveauRecord(lignes), apres)
}

// AddIndividual crée un enregistrement "0 @XREF@ INDI". lignesNiveau1 est le corps
// déjà mis en forme (ex. "1 SEX M", "1 BIRT", "2 DATE 9 MAR 1805"...) — pas de
// NAME/GIVN/SURN implicite : à fournir explicitement pour rester cohérent avec le
// reste du fichier.
func (g *Gedcom) AddIndividual(xref string, lignesNiveau1 []string, apres string) (*Record, error) {
	xref = strings.Trim(xref, "@")
	if g.Contains(xref) {
		return nil, fmt.Errorf("@%s@ existe déjà", xref)
	}
	lignes := append([]string{"0 @" + xref + "@ INDI"}, lignesNiveau1...)
	lignes = append(lignes, ligneChanMaintenant()...)
	return g.inserer(nouveauRecord(lignes), apres)
}

// AddFamily crée un enregistrement "0 @XREF@ FAM" avec HUSB/WIFE/CHIL optionnels
// (husb/wife == "" pour omettre).
func (g *Gedcom) AddFamily(xref, husb, wife string, chil []string, apres string) (*Record, error) {
	xref = strings.Trim(xref, "@")
	if g.Contains(xref) {
		return nil, fmt.Errorf("@%s@ existe déjà", xref)
	}
	lignes := []string{"0 @" + xref + "@ FAM"}
	if husb != "" {
		lignes = append(lignes, "1 HUSB @"+strings.Trim(husb, "@")+"@")
	}
	if wife != "" {
		lignes = append(lignes, "1 WIFE @"+strings.Trim(wife, "@")+"@")
	}
	for _, c := range chil {
		lignes = append(lignes, "1 CHIL @"+strings.Trim(c, "@")+"@")
	}
	lignes = append(lignes, ligneChanMaintenant()...)
	return g.inserer(nouveauRecord(lignes), apres)
}

// ProchainXref renvoie le prochain identifiant libre pour ce préfixe :
// ProchainXref("S") -> "S0008" si S0000..S0007 existent déjà.
func (g *Gedcom) ProchainXref(prefixe string) string {
	re := regexp.MustCompile("^" + regexp.QuoteMeta(prefixe) + `(\d+)$`)
	maxNum, largeur, vu := -1, 4, false
	for xref := range g.ParXref() {
		m := re.FindStringSubmatch(xref)
		if m == nil {
			continue
		}
		if n, err := strconv.Atoi(m[1]); err == nil && n > maxNum {
			maxNum = n
		}
		if !vu || len(m[1]) > largeur {
			largeur = len(m[1])
		}
		vu = true
	}
	return fmt.Sprintf("%s%0*d", prefixe, largeur, maxNum+1)
}

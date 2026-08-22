package geneanet

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// EstFicheGeneanet reconnaît le HTML d'une page de fiche individuelle Geneanet, par
// sniff de contenu plutôt que par extension de fichier (l'utilisateur peut enregistrer
// en .html, .htm, voire .txt selon son navigateur). Toute page Geneanet ("gw", le
// moteur GeneWeb) porte un lien canonique vers gw.geneanet.org, cherché en substring
// brute — pas la peine de parser le HTML juste pour ça.
func EstFicheGeneanet(contenu []byte) bool {
	return bytes.Contains(contenu, []byte("gw.geneanet.org"))
}

// ParserHTML construit une Fiche à partir du HTML d'une page individuelle Geneanet
// (enregistrée par l'utilisateur — "Enregistrer sous" ou "Afficher la source" depuis
// le navigateur). Une section absente de la page laisse le champ correspondant vide
// (nil/zéro) plutôt que d'être devinée — voir Fiche.GrandsParentsPaternels.Enfants et
// Fiche.DemiFratrie, jamais peuplés à partir d'une page qui ne les montre pas.
func ParserHTML(contenu []byte) (*Fiche, error) {
	doc, err := html.Parse(bytes.NewReader(contenu))
	if err != nil {
		return nil, fmt.Errorf("HTML illisible : %w", err)
	}
	tous := aplatir(doc)

	h1 := premier(tous, func(n *html.Node) bool { return estBalise(n, "h1") })
	if h1 == nil {
		return nil, fmt.Errorf("fiche introuvable : aucun <h1> — pas une page individuelle Geneanet")
	}
	h1Idx := indexDe(tous, h1)
	apresH1 := tous[h1Idx:]

	f := &Fiche{}
	if err := extraireSujet(f, h1); err != nil {
		return nil, err
	}

	// Préambule (naissance/décès/profession) : les <li> entre le <h1> et le premier
	// <h2> de section.
	h2Debut := indexPremierH2(apresH1)
	for _, li := range trouverTous(apresH1[:h2Debut], estLi) {
		contenu := texte(li)
		if contenu != "" && !f.parseBulletNaissanceDeces(contenu) {
			f.parseOccupation(contenu)
		}
	}

	extraireParents(f, section(apresH1, "parents"))
	extraireFratrie(f, section(apresH1, "frères et s"))
	extraireUnions(f, section(apresH1, "union"))
	extraireNotesUnion(f, section(apresH1, "notes concernant"))
	extraireGrandsParents(f, section(apresH1, "grands-parents"))
	extraireSources(f, section(apresH1, "sources"))
	extraireNotes(f, section(apresH1, "notes"))

	return f, nil
}

// ------------------------------------------------------------------ helpers de nœud

func estBalise(n *html.Node, tag string) bool { return n.Type == html.ElementNode && n.Data == tag }
func estLi(n *html.Node) bool                 { return estBalise(n, "li") }

func attr(n *html.Node, nom string) string {
	for _, a := range n.Attr {
		if a.Key == nom {
			return a.Val
		}
	}
	return ""
}

func aClasse(n *html.Node, classe string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == classe {
			return true
		}
	}
	return false
}

// texte concatène récursivement le texte de n et ses descendants, espaces (dont
// &nbsp;, unicode.IsSpace le reconnaît déjà) normalisés en un seul espace.
func texte(n *html.Node) string {
	if n == nil {
		return ""
	}
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(sb.String()), " ")
}

// texteSuperficiel concatène le texte de n comme texte, mais s'arrête avant tout
// <ul> descendant — pour lire le texte propre d'une union ("Marié le... avec X dont")
// sans y mélanger le texte de la liste d'enfants qu'elle contient.
func texteSuperficiel(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if estBalise(n, "ul") {
			return
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(sb.String()), " ")
}

func aplatir(n *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		out = append(out, n)
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return out
}

func trouverTous(noeuds []*html.Node, pred func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	for _, n := range noeuds {
		if pred(n) {
			out = append(out, n)
		}
	}
	return out
}

func premier(noeuds []*html.Node, pred func(*html.Node) bool) *html.Node {
	for _, n := range noeuds {
		if pred(n) {
			return n
		}
	}
	return nil
}

// nomNoeud trouve le noeud qui porte le NOM d'une personne dans noeuds (déjà aplatis
// depuis un <li>/<td>) : un <a> ou un <b> (auto-référence, sans lien vers soi-même) —
// mais seulement celui dont le texte n'est pas vide. Une cellule "arbre_pers" avec
// photo porte en effet DEUX <a> : le premier n'entoure qu'une vignette <img> (texte
// vide), le second le nom ; prendre le premier <a> sans filtrer aurait donné un nom
// vide, laissant nomEtAnnees en face d'un millésime seul, sans le mot précédent dont
// il a besoin pour le repérer (confirmé sur données réelles : "Jean Rouquet" avec
// photo donnait un individu nommé "/1685-1754/").
func nomNoeud(noeuds []*html.Node) *html.Node {
	if n := premier(noeuds, func(n *html.Node) bool { return estBalise(n, "a") && texte(n) != "" }); n != nil {
		return n
	}
	return premier(noeuds, func(n *html.Node) bool { return estBalise(n, "b") && texte(n) != "" })
}

func indexDe(noeuds []*html.Node, cible *html.Node) int {
	for i, n := range noeuds {
		if n == cible {
			return i
		}
	}
	return -1
}

func indexPremierH2(noeuds []*html.Node) int {
	for i, n := range noeuds {
		if estBalise(n, "h2") {
			return i
		}
	}
	return len(noeuds)
}

// section extrait la tranche de noeuds (déjà bornée à "après le <h1>") allant du
// premier <h2> dont le texte contient motCle (normalisé, insensible à la casse) au
// <h2> suivant (ou la fin) — robuste aux différences d'emballage <div> selon la
// section (Sources et l'arbre des grands-parents sont chacun dans un <div> en plus,
// Parents/Fratrie non), puisque c'est une tranche par position, pas par ancêtre commun.
func section(noeuds []*html.Node, motCle string) []*html.Node {
	debut := -1
	for i, n := range noeuds {
		if !estBalise(n, "h2") {
			continue
		}
		if strings.Contains(strings.ToLower(texte(n)), motCle) {
			debut = i
			break
		}
	}
	if debut == -1 {
		return nil
	}
	fin := len(noeuds)
	for i := debut + 1; i < len(noeuds); i++ {
		if estBalise(noeuds[i], "h2") {
			fin = i
			break
		}
	}
	return noeuds[debut:fin]
}

// ---------------------------------------------------------------- extraction Sujet

func extraireSujet(f *Fiche, h1 *html.Node) error {
	sexe := ""
	if img := premier(aplatir(h1), func(n *html.Node) bool { return estBalise(n, "img") }); img != nil {
		switch attr(img, "alt") {
		case "Homme":
			sexe = "M"
		case "Femme":
			sexe = "F"
		}
	}
	var mots []string
	for _, a := range trouverTous(aplatir(h1), func(n *html.Node) bool { return estBalise(n, "a") }) {
		if t := texte(a); t != "" {
			mots = append(mots, t)
		}
	}
	if len(mots) == 0 {
		return fmt.Errorf("nom du sujet introuvable dans le <h1>")
	}
	f.Sujet.Nom = strings.Join(mots, " ")
	f.Sujet.Sexe = sexe
	return nil
}

// -------------------------------------------------------- extraction personne (li)

// extrairePersonneLi lit une mention de personne "<img alt?> <a|b>Nom</a|b>
// <bdo>années</bdo>" — forme commune à Parents, Frères et sœurs, enfants d'union, et
// l'arbre des grands-parents (td plutôt que li, même forme interne).
func extrairePersonneLi(li *html.Node) Personne {
	noeuds := aplatir(li)
	sexe := ""
	if img := premier(noeuds, func(n *html.Node) bool { return estBalise(n, "img") }); img != nil {
		switch attr(img, "alt") {
		case "Homme":
			sexe = "M"
		case "Femme":
			sexe = "F"
		}
	}
	bdo := premier(noeuds, func(n *html.Node) bool { return estBalise(n, "bdo") })
	nom, naissance, deces, okN, okD := nomEtAnnees(strings.TrimSpace(texte(nomNoeud(noeuds)) + " " + texte(bdo)))
	return Personne{Nom: nom, Sexe: sexe, Naissance: naissance, Deces: deces, OkNaissance: okN, OkDeces: okD}
}

// ------------------------------------------------------------- extraction Parents

func extraireParents(f *Fiche, noeuds []*html.Node) {
	lis := trouverTous(noeuds, estLi)
	for i, li := range lis {
		if i > 1 {
			break
		}
		f.Parents[i] = extrairePersonneLi(li)
	}
}

// ------------------------------------------------------------------ Frères et sœurs

func extraireFratrie(f *Fiche, noeuds []*html.Node) {
	for _, li := range trouverTous(noeuds, estLi) {
		f.Fratrie = append(f.Fratrie, extrairePersonneLi(li))
	}
}

// -------------------------------------------------------------------------- Unions

// extraireUnions lit chaque <li> direct de <ul class="fiche_union"> : le texte
// superficiel (avant le <ul> imbriqué des enfants) a exactement la forme "Marié(e)
// le <date> (<jour>), <lieu>, avec <conjoint> <années> dont" — réutilise
// parseUnionBullet, déjà conçu pour ce motif. Les enfants sont le <ul> imbriqué,
// même forme que Frères et sœurs.
func extraireUnions(f *Fiche, noeuds []*html.Node) {
	ul := premier(noeuds, func(n *html.Node) bool { return estBalise(n, "ul") && aClasse(n, "fiche_union") })
	if ul == nil {
		return
	}
	for li := ul.FirstChild; li != nil; li = li.NextSibling {
		if !estLi(li) {
			continue
		}
		u := parseUnionBullet(texteSuperficiel(li))
		if sousUl := premier(aplatir(li), func(n *html.Node) bool { return estBalise(n, "ul") }); sousUl != nil {
			for _, enfantLi := range trouverTous(aplatir(sousUl), estLi) {
				u.Enfants = append(u.Enfants, extrairePersonneLi(enfantLi))
			}
		}
		f.Unions = append(f.Unions, u)
	}
}

// -------------------------------------------------------------- Grands-parents

// extraireGrandsParents lit les 4 premières cellules "arbre_pers" de l'arbre
// d'ascendance (grand-père/grand-mère paternels, puis maternels, dans cet ordre —
// confirmé sur du HTML réel). Enfants (oncles/tantes) reste toujours vide : cette vue
// ne montre que les 4 grands-parents directs, jamais la fratrie de chacun.
func extraireGrandsParents(f *Fiche, noeuds []*html.Node) {
	cellules := trouverTous(noeuds, func(n *html.Node) bool { return estBalise(n, "td") && aClasse(n, "arbre_pers") })
	if len(cellules) < 4 {
		return
	}
	lire := func(td *html.Node) Personne {
		aplati := aplatir(td)
		bdo := premier(aplati, func(n *html.Node) bool { return estBalise(n, "bdo") })
		nom, naissance, deces, okN, okD := nomEtAnnees(strings.TrimSpace(texte(nomNoeud(aplati)) + " " + texte(bdo)))
		return Personne{Nom: nom, Naissance: naissance, Deces: deces, OkNaissance: okN, OkDeces: okD}
	}
	f.GrandsParentsPaternels = &GrandParentGroupe{GrandPere: lire(cellules[0]), GrandMere: lire(cellules[1])}
	f.GrandsParentsMaternels = &GrandParentGroupe{GrandPere: lire(cellules[2]), GrandMere: lire(cellules[3])}
}

// ------------------------------------------------------------------------- Sources

func extraireSources(f *Fiche, noeuds []*html.Node) {
	for _, li := range trouverTous(noeuds, estLi) {
		if t := texte(li); t != "" {
			f.Sources = append(f.Sources, parseSource(t))
		}
	}
}

// --------------------------------------------------------------------------- Notes

// extraireNotes lit la section "Notes" (individuelle) : une paire <h3
// class="note_type">Type</h3> + <div class="fiche-note-ind">texte</div> par note,
// écrites "Type : texte" dans Fiche.Notes. Les notes de la section "Notes concernant
// l'union" (une par union, div.fiche-note-union) sont associées à Fiche.Unions par
// ordre d'apparition — appelée séparément, après extraireUnions, sur toute la page
// (pas seulement la tranche "notes", cette section a son propre <h2>).
func extraireNotes(f *Fiche, noeudsNotes []*html.Node) {
	for _, div := range trouverTous(noeudsNotes, func(n *html.Node) bool { return estBalise(n, "div") && aClasse(n, "fiche-note-ind") }) {
		texteNote := texte(div)
		if texteNote == "" {
			continue
		}
		label := texte(h3Precedent(noeudsNotes, div))
		if label != "" {
			texteNote = label + " : " + texteNote
		}
		f.Notes = append(f.Notes, texteNote)
	}
}

// extraireNotesUnion associe chaque div.fiche-note-union de la section "Notes
// concernant l'union" à l'union correspondante, par ordre d'apparition.
func extraireNotesUnion(f *Fiche, noeuds []*html.Node) {
	divs := trouverTous(noeuds, func(n *html.Node) bool { return estBalise(n, "div") && aClasse(n, "fiche-note-union") })
	for i, div := range divs {
		if i >= len(f.Unions) {
			break
		}
		if t := texte(div); t != "" {
			f.Unions[i].Note = t
		}
	}
}

// h3Precedent renvoie le dernier <h3> rencontré avant cible dans noeuds (ordre
// document) — associe une note à son type ("Naissance", "Décès"...).
func h3Precedent(noeuds []*html.Node, cible *html.Node) *html.Node {
	var dernier *html.Node
	for _, n := range noeuds {
		if n == cible {
			return dernier
		}
		if estBalise(n, "h3") {
			dernier = n
		}
	}
	return dernier
}

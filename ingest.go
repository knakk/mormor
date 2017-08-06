package main

import (
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/knakk/kbp/marc"
	"github.com/knakk/kbp/marc/marc21"
	"github.com/knakk/kbp/rdf"
	"github.com/knakk/kbp/rdf/memory"
)

type source uint

const (
	sourceOria source = iota
	sourceNasjonalbiblioteket
	sourceGoogle
	sourceLibraryOfCongress
	sourceLibraryThing
	sourceOpenLibrary
)

func ingestPublication(id rdf.NamedNode, input io.Reader, s source) (*memory.Graph, error) {
	switch s {
	case sourceOria:
		return ingestOria(id, input)
	default:
		panic("ingestPublication: TODO")
	}
}

func ingestOria(id rdf.NamedNode, input io.Reader) (*memory.Graph, error) {
	rec, err := marc.NewDecoder(input, marc.MARCXML).Decode()
	if err != nil {
		return nil, err
	}
	g := memory.NewGraph()

	// Publication class
	g.Insert(rdf.Triple{id, rdf.RDFtype, rdf.NewNamedNode("Publication")})

	// Publication ISBN and binding (soft/hardcover)
	for _, f := range rec.DataFields(marc.Tag020) {
		for _, sf := range f.Subfield('q') {
			switch sf {
			case "ib.", "ib":
				g.Insert(rdf.Triple{id, rdf.NewNamedNode("hasBinding"), rdf.NewNamedNode("binding/hardback")})
			case "h":
				g.Insert(rdf.Triple{id, rdf.NewNamedNode("hasBinding"), rdf.NewNamedNode("binding/paperback")})
			}
			break
		}
		for _, sf := range f.Subfield('a') {
			g.Insert(rdf.Triple{id, rdf.NewNamedNode("hasISBN"), rdf.NewStrLiteral(sf)})
			break
		}
		break
	}

	// Publication number of pages
	if numPages := marcField(rec, marc.Tag300, 'a'); numPages != "" {
		g.Insert(rdf.Triple{id, rdf.NewNamedNode("hasNumPages"), rdf.NewTypedLiteral(cleanNumber(numPages), rdf.XSDint)})
	}

	title := marcField(rec, marc.Tag245, 'a')

	var lang string
	if cf, ok := rec.ControlField(marc.Tag008); ok {
		lang = strings.TrimSpace(cf.GetPos(marc21.C008Language.Pos, marc21.C008Language.Length))
	}

	work := rdf.NewBlankNode("work")
	g.Insert(
		rdf.Triple{id, rdf.NewNamedNode("isPublicationOf"), work},
		rdf.Triple{work, rdf.RDFtype, rdf.NewNamedNode("Work")})
	if title != "" {
		g.Insert(
			rdf.Triple{id, rdf.NewNamedNode("hasMainTitle"), rdf.NewStrLiteral(title)})
		if lang != "" {
			g.Insert(
				rdf.Triple{work, rdf.NewNamedNode("hasName"), rdf.NewLangLiteral(title, lang)},
				rdf.Triple{work, rdf.NewNamedNode("hasLanguage"), rdf.NewNamedNode("lang/" + lang)})
		} else {
			g.Insert(rdf.Triple{work, rdf.NewNamedNode("hasName"), rdf.NewStrLiteral(title)})
		}
	}

	// Is work a translation?
	isTranslation := false
	var origWork rdf.BlankNode
	for _, f := range rec.DataFields(marc.Tag246) {
		for _, s := range f.Subfield('i') {
			if s == "Originaltittel" || s == "originaltittel" {
				isTranslation = true
				origWork = rdf.NewBlankNode("origWork")
				g.Insert(
					rdf.Triple{work, rdf.NewNamedNode("isTranslationOf"), origWork},
					rdf.Triple{origWork, rdf.RDFtype, rdf.NewNamedNode("Work")})
				if len(f.Subfield('a')) > 0 {
					g.Insert(rdf.Triple{origWork, rdf.NewNamedNode("hasName"), rdf.NewStrLiteral(f.Subfield('a')[0])})
				}
			}
		}
	}

	// Work main entry
	for _, f := range rec.DataFields(marc.Tag100) {
		contrib := rdf.NewBlankNode("mainEntryContrib")
		agent := rdf.NewBlankNode("mainEntryAgent")
		for _, s := range f.Subfield('a') {
			g.Insert(
				rdf.Triple{contrib, rdf.RDFtype, rdf.NewNamedNode("Contribution")},
				rdf.Triple{contrib, rdf.NewNamedNode("hasRole"), rdf.NewNamedNode("role/author")},
				rdf.Triple{contrib, rdf.NewNamedNode("hasAgent"), agent},
				rdf.Triple{agent, rdf.RDFtype, rdf.NewNamedNode("Person")},
				rdf.Triple{agent, rdf.NewNamedNode("hasName"), rdf.NewStrLiteral(reinvertName(s))})
		}
		for _, s := range f.Subfield('d') {
			years := strings.Split(s, "-")
			birthDate := rdf.NewBlankNode("mainEntryAgentBirthDate")
			g.Insert(
				rdf.Triple{agent, rdf.NewNamedNode("hasBirthDate"), birthDate},
				rdf.Triple{birthDate, rdf.RDFtype, rdf.NewNamedNode("Date")},
				rdf.Triple{birthDate, rdf.NewNamedNode("hasYear"), rdf.NewTypedLiteral(cleanNumber(years[0]), rdf.XSDint)})
			if len(years) == 2 && len(years[1]) > 0 {
				deathDate := rdf.NewBlankNode("mainEntryAgentDeathDate")
				g.Insert(
					rdf.Triple{agent, rdf.NewNamedNode("hasDeathDate"), deathDate},
					rdf.Triple{deathDate, rdf.RDFtype, rdf.NewNamedNode("Date")},
					rdf.Triple{deathDate, rdf.NewNamedNode("hasYear"), rdf.NewTypedLiteral(cleanNumber(years[1]), rdf.XSDint)},
				)
			}
		}

		if isTranslation {
			g.Insert(rdf.Triple{origWork, rdf.NewNamedNode("hasContribution"), contrib})
		} else {
			g.Insert(rdf.Triple{work, rdf.NewNamedNode("hasContribution"), contrib})
		}
	}

	// Other contributions
	for i, f := range rec.DataFields(marc.Tag700) {
		contrib := rdf.NewBlankNode("contrib" + strconv.Itoa(i))
		agent := rdf.NewBlankNode("agent" + strconv.Itoa(i))
		for _, s := range f.Subfield('a') {
			g.Insert(
				rdf.Triple{contrib, rdf.NewNamedNode("hasAgent"), agent},
				rdf.Triple{agent, rdf.RDFtype, rdf.NewNamedNode("Person")},
				rdf.Triple{agent, rdf.NewNamedNode("hasName"), rdf.NewStrLiteral(reinvertName(s))})
		}
		g.Insert(
			rdf.Triple{work, rdf.NewNamedNode("hasContribution"), contrib},
			rdf.Triple{contrib, rdf.RDFtype, rdf.NewNamedNode("Contribution")})
	}

	// Publication publisher and publish-year
	for _, f := range rec.DataFields(marc.Tag260) {
		for _, sf := range f.Subfield('c') {
			g.Insert(rdf.Triple{id, rdf.NewNamedNode("hasPublishYear"), rdf.NewTypedLiteral(sf, rdf.XSDint)})
			break
		}
		for _, sf := range f.Subfield('b') {
			bNode := rdf.NewBlankNode("publisher")
			g.Insert(
				rdf.Triple{id, rdf.NewNamedNode("hasPublisher"), bNode},
				rdf.Triple{bNode, rdf.RDFtype, rdf.NewNamedNode("Publisher")},
				rdf.Triple{bNode, rdf.NewNamedNode("hasName"), rdf.NewStrLiteral(sf)})
			break
		}
		break
	}

	// Work literary form
	for _, s := range marcFields(rec, marc.Tag653, 'a') {
		if s == "roman" {
			g.Insert(
				rdf.Triple{work, rdf.NewNamedNode("hasLiteraryForm"), rdf.NewNamedNode("form/novel")})
		}
	}
	// TODO C008 Pos 33

	return g, nil
}

func marcField(r *marc.Record, tag marc.DataTag, subfield rune) string {
	for _, f := range r.DataFields(tag) {
		for _, sf := range f.Subfield(subfield) {
			return sf
		}
	}
	return ""
}

func marcFields(r *marc.Record, tag marc.DataTag, subfield rune) (res []string) {
	for _, f := range r.DataFields(tag) {
		res = append(res, f.Subfield(subfield)...)
	}
	return res
}

var reDigits = regexp.MustCompile("[0-9]+")

func cleanNumber(s string) string {
	return reDigits.FindString(s)
}

func reinvertName(s string) string {
	if i := strings.Index(s, ","); i > 0 {
		// TODO make it safe from index out of bounds panic
		return s[i+2:] + " " + s[:i]
	}
	return s
}

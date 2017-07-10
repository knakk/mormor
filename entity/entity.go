package entity

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"strconv"
	"strings"

	"github.com/knakk/kbp/rdf"
)

// Entity represents an entity.
type Entity interface {

	// ID is the URI of the entity
	ID() string

	// CanonicalTtile is the canonical title/name of the entity.
	CanonicalTitle() string

	// Abstract returns a textual representation of the entity.
	Abstract() string

	// EntityType returns the Type of the entity
	EntityType() Type

	// Process performs transformations on the entity object. Usefull for
	// extracting and structuring data in ways not possible using rdf struct tags.
	Process()
}

// Type represents the type of an entity.
type Type uint

// Available Types:
const (
	typeInvalid Type = iota
	TypePerson
	TypeCorporation
	TypePublication
	TypeWork
	TypePublisherSeries
)

func (t Type) Class() rdf.NamedNode {
	switch t {
	case TypePerson:
		return rdf.NewNamedNode("Person")
	case TypeCorporation:
		return rdf.NewNamedNode("Corporation")
	case TypePublication:
		return rdf.NewNamedNode("Publication")
	case TypeWork:
		return rdf.NewNamedNode("Work")
	case TypePublisherSeries:
		return rdf.NewNamedNode("PublisherSeries")
	case typeInvalid:
		fallthrough
	default:
		return rdf.NewNamedNode("invalid entity.Type")
	}
}

// String returns a string representation of a Type.
func (t Type) String() string {
	switch t {
	case TypePerson:
		return "Person"
	case TypeCorporation:
		return "Corporation"
	case TypePublication:
		return "Publication"
	case TypeWork:
		return "Work"
	case TypePublisherSeries:
		return "Publisher series"
	case typeInvalid:
		fallthrough
	default:
		return "invalid entity.Type"
	}
}

func TypeFromURI(uri rdf.NamedNode) Type {
	i := strings.Index(uri.Name(), "/")
	if i < 0 {
		return typeInvalid
	}
	switch uri.Name()[:i] {
	case "person":
		return TypePerson
	case "corporation":
		return TypeCorporation
	case "publication":
		return TypePublication
	case "work":
		return TypeWork
	default:
		return typeInvalid
	}
}

type Person struct {
	URI              string                 `rdf:"id"`
	Name             string                 `rdf:"->hasName"`
	BirthDate        *Date                  `rdf:"->hasBirthDate"`
	DeathDate        *Date                  `rdf:"->hasDeathDate"`
	ShortDescription string                 `rdf:"->hasShortDescription"`
	LongDescription  string                 `rdf:"->hasDescription;->hasText"`
	Links            []string               `rdf:">>hasLink"`
	Works            []WorkWithPublications `rdf:"<<hasAgent;<-hasContribution"`
	// ^ TODO rename OriginalWorks ?
	Compilations []WorkWithPublications
	WorksAbout   []WorkWithPublications `rdf:"<<hasSubject"`
}

type Date struct {
	Year      int  `rdf:"->hasYear"`
	YearLower int  `rdf:"->hasYearLower"`
	YearUpper int  `rdf:"->hasYearUpper"`
	Approx    bool `rdf:"->isApproximate"`
}

func (d Date) String() string {
	if d.Year != 0 {
		if d.Approx {
			return "ca. " + strconv.Itoa(d.Year)
		}
		return strconv.Itoa(d.Year)
	}
	if d.YearLower != 0 {
		if d.YearUpper-d.YearLower == 1 {
			return strconv.Itoa(d.YearLower) + "/" + strconv.Itoa(d.YearUpper)
		}
		if d.YearUpper != 0 {
			return strconv.Itoa(d.YearLower) + "-" + strconv.Itoa(d.YearUpper)
		}
		return "ca. " + strconv.Itoa(d.YearLower)
	}
	return ""
}

type Work struct {
	URI                  string                   `rdf:"id"`
	OriginalTitle        string                   `rdf:"->hasOriginalTitle"`
	Title                string                   `rdf:"@>hasTitle"`
	AltTitle             []string                 `rdf:">>hasAlternativeTitle"`
	Language             namedWithLang            `rdf:"->hasLanguage"`
	Contributions        []contribution           `rdf:">>hasContribution"`
	TranslationOf        *WorkWithoutTranslations `rdf:"->isTranslationOf"`
	FirstPublicationDate *Date                    `rdf:"->hasFirstPublicationDate"`
	Forms                []string                 `rdf:">>hasLiteraryForm;@>hasName"`
	Compilation          bool                     `rdf:"->isCompilation"`
}

type WorkWithPublications struct {
	Work
	Translations []WorkWithPublications `rdf:"<<isTranslationOf"`
	Publications []Publication          `rdf:"<<isPublicationOf"`
}

type WorkWithoutTranslations struct {
	Work
	Publications []Publication `rdf:"<<isPublicationOf"`
}

type contribution struct {
	Role  string `rdf:"->hasRole"`
	Agent named  `rdf:"->hasAgent"`
	Alias string `rdf:"->usingPseudonym;->hasName"`
}

type PublicationWithWork struct {
	Publication
	Work Work `rdf:"->isPublicationOf"`
}

type Publication struct {
	URI              string        `rdf:"id"`
	Title            string        `rdf:"->hasMainTitle"`
	Subtitle         string        `rdf:"->hasSubtitle"`
	PublishYear      int           `rdf:"->hasPublishYear"`
	Publisher        named         `rdf:"->hasPublisher"`
	PublicationPlace string        `rdf:"->hasPubliationPlace;->hasName"`
	Binding          string        `rdf:"->hasBinding;->hasName"`
	NumPages         int           `rdf:"->hasNumPages"`
	Image            string        `rdf:"->hasImage"`
	ISBN             []string      `rdf:">>hasISBN"`
	Description      template.HTML `rdf:"->hasPublisherDescription"`
	EditionNote      string        `rdf:"->hasEditionNote"`
	Series           []SeriesEntry `rdf:">>isPublishedInSeries"`
}

type SeriesEntry struct {
	NumberInSeries int   `rdf:"->hasNumber"`
	Series         named `rdf:"->inSeries"`
}

func (p *Publication) NumberInSeries(series string) int {
	for _, s := range p.Series {
		if s.Series.Name == series {
			return s.NumberInSeries
		}
	}
	return 0
}

type named struct {
	URI  string `rdf:"id"`
	Name string `rdf:"->hasName"`
}

type namedWithLang struct {
	URI  string `rdf:"id"`
	Name string `rdf:"@>hasName"`
}

type PublisherSeries struct {
	URI              string `rdf:"id"`
	Name             string `rdf:"->hasName"`
	ShortDescription string `rdf:"->hasShortDescription"`
	Publisher        named
	Contributions    []contribution        `rdf:">>hasContribution"`
	Publications     []PublicationWithWork `rdf:"<<inSeries;<-isPublishedInSeries"`
}

func (p *PublisherSeries) ID() string       { return p.URI }
func (p *PublisherSeries) Abstract() string { return "" }
func (p *PublisherSeries) EntityType() Type { return TypePublisherSeries }
func (p *PublisherSeries) Process()         {}

func (p *Person) CanonicalTitle() string {
	var b bytes.Buffer
	if err := tmplPersonTitle.Execute(&b, p); err != nil {
		return err.Error()
	}
	return b.String()
}

func (p *Person) ID() string       { return p.URI }
func (p *Person) Abstract() string { return p.ShortDescription }
func (p *Person) EntityType() Type { return TypePerson }
func (p *Person) Process() {
	for i := 0; i < len(p.Works); i++ {
		if p.Works[i].Compilation {
			p.Compilations = append(p.Compilations, p.Works[i])
			p.Works = append(p.Works[:i], p.Works[i+1:]...)
			i--
		}
	}
	sort.Slice(p.Works, func(i, j int) bool {
		return p.Works[i].FirstPublicationDate != nil &&
			p.Works[j].FirstPublicationDate != nil &&
			p.Works[i].FirstPublicationDate.Year > p.Works[j].FirstPublicationDate.Year
	})
}

func (p *Person) WorksOriginal() (res []WorkWithPublications) {
	for _, w := range p.Works {
		if !w.IsTranslation() {
			res = append(res, w)
		}
	}
	return res
}

func (p *Person) WorksAs(role string, uri string) (res []WorkWithPublications) {
	for _, w := range p.Works {
		for _, c := range w.Contributions {
			if c.Role == role && c.Agent.URI == uri {
				res = append(res, w)
			}
		}
	}
	return res
}

type Link struct {
	Target string
	Label  string
}

func (w *WorkWithPublications) PublicationLinksByLanguageExcluding(langs []string) map[string][]Link {
	res := make(map[string][]Link)
	if len(w.Publications) > 0 {
		found := false
		for _, lang := range langs {
			if w.Language.URI == lang {
				found = true
				break
			}
		}
		if !found {
			for _, p := range w.Publications {
				res[w.Language.Name] = append(res[w.Language.Name], Link{
					Target: fmt.Sprintf("/%s/%s", w.URI, p.URI),
					Label:  strconv.Itoa(p.PublishYear),
				})
			}
		}
	}
	for _, w := range w.Translations {
		found := false
		if len(w.Publications) > 0 {
			for _, lang := range langs {
				if w.Language.URI == lang {
					found = true
					break
				}
			}
		}
		if !found {
			for _, p := range w.Publications {
				res[w.Language.Name] = append(res[w.Language.Name], Link{
					Target: fmt.Sprintf("/%s/%s", w.URI, p.URI),
					Label:  strconv.Itoa(p.PublishYear),
				})
			}
		}
	}

	for lang, _ := range res {
		sort.Slice(res[lang], func(i, j int) bool {
			return res[lang][i].Label < res[lang][j].Label
		})
	}
	return res
}

func (w *WorkWithPublications) PublicationLinksForLanguages(langs []string) []Link {
	var res []Link
	if len(w.Publications) > 0 {
		for _, lang := range langs {
			if w.Language.URI == lang {
				for _, p := range w.Publications {
					res = append(res, Link{
						Target: fmt.Sprintf("/%s/%s", w.URI, p.URI),
						Label:  strconv.Itoa(p.PublishYear),
					})
				}
			}
		}
	}
	for _, w := range w.Translations {
		if len(w.Publications) > 0 {
			for _, lang := range langs {
				if w.Language.URI == lang {
					for _, p := range w.Publications {
						res = append(res, Link{
							Target: fmt.Sprintf("/%s/%s", w.URI, p.URI),
							Label:  strconv.Itoa(p.PublishYear),
						})
					}
				}
			}
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Label < res[j].Label
	})
	return res
}

func (w *WorkWithPublications) TranslationsByLanguage() map[namedWithLang][]WorkWithPublications {
	if len(w.Translations) == 0 {
		return nil
	}
	res := make(map[namedWithLang][]WorkWithPublications)
	for _, t := range w.Translations {
		res[t.Language] = append(res[t.Language], t)
	}
	return res
}

func (w *Work) ID() string       { return w.URI }
func (w *Work) EntityType() Type { return TypeWork }
func (w *Work) Abstract() string { return "" }

func (w *Work) CanonicalTitle() string {
	var b bytes.Buffer
	if err := tmplWorkTitle.Execute(&b, w); err != nil {
		return err.Error()
	}
	return b.String()
}

func (w *Work) Process() {}

func (w *Work) ContribsBy(role string) (c []contribution) {
	for _, contrib := range w.Contributions {
		if contrib.Role == role {
			c = append(c, contrib)
		}
	}
	if w.TranslationOf != nil {
		for _, contrib := range w.TranslationOf.Contributions {
			if contrib.Role == role {
				c = append(c, contrib)
			}
		}
	}
	return c
}

func (w *Work) IsTranslation() bool { return w.TranslationOf != nil }

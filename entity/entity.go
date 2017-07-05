package entity

import (
	"bytes"
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
	Title                string                   `rdf:"->hasTitle"`
	AltTitle             []string                 `rdf:">>hasAlternativeTitle"`
	Language             string                   `rdf:"->hasLanguage"`
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
	Agent agent  `rdf:"->hasAgent"`
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
	Publisher        agent         `rdf:"->hasPublisher"`
	PublicationPlace string        `rdf:"->hasPubliationPlace;->hasName"`
	Binding          string        `rdf:"->hasBinding;->hasName"`
	NumPages         int           `rdf:"->hasNumPages"`
	Image            string        `rdf:"->hasImage"`
	ISBN             []string      `rdf:">>hasISBN"`
	Description      template.HTML `rdf:"->hasPublisherDescription"`
	EditionNote      string        `rdf:"->hasEditionNote"`
}

type agent struct {
	URI  string `rdf:"id"`
	Name string `rdf:"->hasName"`
}

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
		return p.Works[i].FirstPublicationDate.Year > p.Works[j].FirstPublicationDate.Year
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

func (w *WorkWithPublications) TranslationsLanguages() map[string]int {
	if len(w.Translations) == 0 {
		return nil
	}
	res := make(map[string]int)
	for _, t := range w.Translations {
		res[t.Language]++
	}
	return res
}
func (w *WorkWithPublications) TranslationsByLanguage() map[string][]WorkWithPublications {
	if len(w.Translations) == 0 {
		return nil
	}
	res := make(map[string][]WorkWithPublications)
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

func (w *Work) Process() {
	/*for _, c := range w.OriginalContributions {
		w.Contributions = append(w.Contributions, c)
	}*/
}
func (w *Work) ContribsBy(role string) (c []contribution) {
	for _, contrib := range w.Contributions {
		if contrib.Role == role {
			c = append(c, contrib)
		}
	}
	return c
}

func (w *Work) IsTranslation() bool { return w.TranslationOf != nil }

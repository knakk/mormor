package entity

import (
	"html/template"
	"sort"
	"strconv"
	"strings"

	"github.com/knakk/kbp/rdf"
)

type Entity interface {
	ID() string
	CanonicalTitle() string
	Abstract() string
	EntityType() Type
	// Process performs any transformations on the entity object
	Process()
}

type Type uint

const (
	typeInvalid Type = iota
	TypePerson
	TypeCorporation
	TypePublication
	TypeWork
)

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
	BirthYear        int                    `rdf:"->hasBirthYear"`
	DeathYear        int                    `rdf:"->hasDeathYear"`
	ShortDescription string                 `rdf:"->hasShortDescription"`
	LongDescription  string                 `rdf:"->hasDescription;->hasText"`
	Links            []string               `rdf:">>hasLink"`
	Works            []WorkWithPublications `rdf:"<<hasAgent;<-hasContribution"`
	OriginalWorks    []WorkWithPublications
	Translations     []WorkWithPublications
	Collections      []WorkWithPublications
	WorksAbout       []WorkWithPublications `rdf:"<<hasSubject"`
}

type Work struct {
	URI                   string         `rdf:"id"`
	Contributions         []contribution `rdf:">>hasContribution"`
	Type                  string         `rdf:"->http://www.w3.org/1999/02/22-rdf-syntax-ns#type"`
	OriginalTitle         string         `rdf:"->isTranslationOf;->hasMainTitle"`
	OriginalAuthors       []agent
	OriginalContributions []contribution `rdf:"->isTranslationOf;>>hasContribution"`
	Authors               []agent
	Alias                 string
	Title                 string   `rdf:"->hasMainTitle"`
	FirstPubYear          int      `rdf:"->hasFirstPublicationYear"`
	Forms                 []string `rdf:"->hasLiteraryForm;->hasName"`
	OriginalForms         []string `rdf:"->isTranslationOf;->hasLiteraryForm;->hasName"`
}

type WorkWithPublications struct {
	URI                   string         `rdf:"id"`
	Contributions         []contribution `rdf:">>hasContribution"`
	Type                  string         `rdf:"->http://www.w3.org/1999/02/22-rdf-syntax-ns#type"`
	OriginalTitle         string         `rdf:"->isTranslationOf;->hasMainTitle"`
	OriginalAuthors       []agent
	OriginalContributions []contribution `rdf:"->isTranslationOf;>>hasContribution"`
	Authors               []agent
	Alias                 string
	Title                 string        `rdf:"->hasMainTitle"`
	FirstPubYear          int           `rdf:"->hasFirstPublicationYear"`
	Forms                 []string      `rdf:"->hasLiteraryForm;->hasName"`
	OriginalForms         []string      `rdf:"->isTranslationOf;->hasLiteraryForm;->hasName"`
	Publications          []Publication `rdf:"<<isPublicationOf"`
}

type contribution struct {
	Role  string `rdf:"->hasRole;->hasName"`
	Agent agent  `rdf:"->hasAgent"`
	Alias string `rdf:"->usingPseudonym;->hasName"`
}

type PublicationWithWork struct {
	URI              string        `rdf:"id"`
	Title            string        `rdf:"->hasMainTitle"`
	Subtitle         string        `rdf:"->hasSubtitle"`
	PubYear          int           `rdf:"->hasPublicationYear"`
	Publisher        agent         `rdf:"->hasPublisher"`
	PublicationPlace string        `rdf:"->hasPubliationPlace;->hasName"`
	Binding          string        `rdf:"->hasBinding;->hasName"`
	NumPages         uint          `rdf:"->hasNumPages"`
	Image            string        `rdf:"->hasImage"`
	Description      template.HTML `rdf:"->hasPublisherDescription"`
	EditionNote      string        `rdf:"->hasEditionNote"`
	Work             Work          `rdf:"->isPublicationOf"`
}

type Publication struct {
	URI              string        `rdf:"id"`
	Title            string        `rdf:"->hasMainTitle"`
	Subtitle         string        `rdf:"->hasSubtitle"`
	PubYear          int           `rdf:"->hasPublicationYear"`
	Publisher        agent         `rdf:"->hasPublisher"`
	PublicationPlace string        `rdf:"->hasPubliationPlace;->hasName"`
	Binding          string        `rdf:"->hasBinding;->hasName"`
	NumPages         uint          `rdf:"->hasNumPages"`
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
	title := p.Name
	if p.BirthYear > 0 || p.DeathYear > 0 {
		title += " ("
		if p.BirthYear > 0 {
			title += strconv.Itoa(p.BirthYear)
		}
		title += "-"
		if p.DeathYear > 0 {
			title += strconv.Itoa(p.DeathYear)
		}
		title += ")"
	}
	return title
}

func (p *Person) ID() string       { return p.URI }
func (p *Person) Abstract() string { return "" }
func (p *Person) EntityType() Type { return TypePerson }
func (p *Person) Process() {
	for _, work := range p.Works {
		switch work.Type {
		case "OriginalWork":
			for _, contrib := range work.Contributions {
				if contrib.Role == "forfatter" && contrib.Agent.URI != p.ID() {
					work.Authors = append(work.Authors, contrib.Agent)
					if contrib.Alias != "" {
						work.Alias = contrib.Alias
					}
				}
			}
			p.OriginalWorks = append(p.OriginalWorks, work)
		case "CollectionWork":
			p.Collections = append(p.Collections, work)
		case "TranslationWork":
			for _, contrib := range work.OriginalContributions {
				if contrib.Role == "forfatter" {
					work.OriginalAuthors = append(work.OriginalAuthors, contrib.Agent)
				}
			}
			p.Translations = append(p.Translations, work)
		}
	}
	sort.Slice(p.OriginalWorks, func(i, j int) bool {
		return p.OriginalWorks[i].FirstPubYear < p.OriginalWorks[j].FirstPubYear
	})
	for y, _ := range p.OriginalWorks {
		sort.Slice(p.OriginalWorks[y].Publications, func(i, j int) bool {
			return p.OriginalWorks[y].Publications[i].PubYear > p.OriginalWorks[y].Publications[j].PubYear
		})
	}
	for i, work := range p.WorksAbout {
		for _, contrib := range work.Contributions {
			if contrib.Role == "forfatter" {
				p.WorksAbout[i].Authors = append(p.WorksAbout[i].Authors, contrib.Agent)
			}
		}
	}
}

func (w *Work) ID() string       { return w.URI }
func (w *Work) EntityType() Type { return TypeWork }
func (w *Work) Abstract() string { return "" }

func (w *Work) CanonicalTitle() string {
	title := w.Title

	var authors []string
	for _, contrib := range w.Contributions {
		authors = append(authors, contrib.Agent.Name)
	}
	if len(authors) > 0 {
		return strings.Join(authors, ",") + ": " + title
	}
	return title
}

func (w *Work) Process() {
	for _, contrib := range w.Contributions {
		if contrib.Role == "forfatter" {
			w.Authors = append(w.Authors, contrib.Agent)
			if contrib.Alias != "" {
				w.Alias = contrib.Alias
			}
		}
	}
}

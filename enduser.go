package main

import (
	"html/template"
	"log"
	"net/http"
	"sort"

	"github.com/knakk/kbp/rdf"
	"github.com/knakk/kbp/rdf/memory"
)

type enduserService struct {
	addr      string
	metadata  *metadataService
	templates *template.Template
}

func newEndUserService(addr string, metadata *metadataService) *enduserService {
	templates := template.Must(template.New("Person").Parse(tmplPerson))
	return &enduserService{
		addr:      addr,
		metadata:  metadata,
		templates: templates,
	}
}

func (e *enduserService) String() string { return "end-user" }

func (e *enduserService) Start() error {
	log.Printf("starting end-user service listening at %s", e.addr)
	return http.ListenAndServe(e.addr, e)
}

func (e *enduserService) Stop() error {
	log.Println("shutting down end-user service")

	return nil
}

func (e *enduserService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if len(r.URL.Path) < 2 {
		http.NotFound(w, r)
		return
	}

	uri := rdf.NewNamedNode(r.URL.Path[1:])
	g, err := e.metadata.triplestore.Describe(rdf.DescSymmetricRecursive, uri)
	if err != nil {
		log.Printf("%s desribe resource error: %v", r.URL.Path, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	res, _ := g.Select([]rdf.Variable{rdf.NewVariable("type")}, rdf.TriplePattern{uri, rdf.RDFtype, rdf.NewVariable("type")})
	if len(res) == 0 {
		http.NotFound(w, r)
		return
	}
	switch res[0][0].(rdf.NamedNode).Name() {
	case "Publication":
	case "Work":
	case "Corporation":
	case "Person":
		var p person
		if err := g.(*memory.Graph).Decode(&p, uri, rdf.NewNamedNode("")); err != nil {
			log.Printf("%s decode Person error: %v", r.URL.Path, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		for _, work := range p.Works {
			switch work.Type {
			case "OriginalWork":
				for _, contrib := range work.Contributions {
					if contrib.Role == "forfatter" {
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
		for i, work := range p.WorksAbout {
			for _, contrib := range work.Contributions {
				if contrib.Role == "forfatter" {
					p.WorksAbout[i].Authors = append(p.WorksAbout[i].Authors, contrib.Agent)
				}
			}
		}
		if err := e.templates.Execute(w, p); err != nil {
			log.Printf("%s Person template error: %v", r.URL.Path, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
}

type person struct {
	Name             string   `rdf:"->hasName"`
	BirthYear        int      `rdf:"->hasBirthYear"`
	DeathYear        int      `rdf:"->hasDeathYear"`
	ShortDescription string   `rdf:"->hasShortDescription"`
	LongDescription  string   `rdf:"->hasDescription;->hasText"`
	Links            []string `rdf:">>hasLink"`
	Works            []work   `rdf:"<<hasAgent;<-hasContribution"`
	OriginalWorks    []work
	Translations     []work
	Collections      []work
	WorksAbout       []work `rdf:"<<hasSubject"`
}

type work struct {
	ID                    string         `rdf:"id"`
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

type contribution struct {
	Role  string `rdf:"->hasRole;->hasName"`
	Agent agent  `rdf:"->hasAgent"`
	Alias string `rdf:"->usingPseudonym;->hasName"`
}

type agent struct {
	ID   string `rdf:"id"`
	Name string `rdf:"->hasName"`
}

const (
	tmplPerson = `
<!doctype html>
<html lang="en">
<head>
	<meta charset=utf-8>
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>{{.Name}} {{if (or (gt .BirthYear 0) (gt .DeathYear 0))}}({{if (gt .BirthYear 0)}}{{.BirthYear}}{{end}}-{{if (gt .DeathYear 0)}}{{.DeathYear}}{{end}}){{end}}</title>
	<style>
		*    { box-sizing: border-box }
		html { font-family: Arial,sans-serif; line-height:1.15; -ms-text-size-adjust: 100%; -webkit-text-size-adjust: 100% }
		body { margin: 0; }
		main { max-width: 980px; margin: auto; padding-top: 2em; }

		td { padding: 0 2em 0.7em 0;}
		td { vertical-align: top; }

		a:link,
		a:visited { color: blue; }

		.tag     { font-size: smaller; color: #777; text-transform: uppercase; background: #eee; border-radius: 3px; padding: 3px;}
		.smaller {color: #888; font-size: smaller;}
	</style>
</head>
<body>
	<main>
		<h1>{{.Name}} {{if (or (gt .BirthYear 0) (gt .DeathYear 0))}}({{if (gt .BirthYear 0)}}{{.BirthYear}}{{end}}-{{if (gt .DeathYear 0)}}{{.DeathYear}}{{end}}){{end}}{{if .ShortDescription}}<br/><span class="smaller">{{.ShortDescription}}</span>{{end}}</h1>
		<p>{{.LongDescription}}</p>

		{{if .Works}}
		<h2>Litterær produksjon</h2>
		{{if .OriginalWorks}}
		<h3>Originalverk</h3>
		<table class="original-work-list">
			<tbody>
				{{- range .OriginalWorks}}
				<tr>
					<td>{{.FirstPubYear}}</td>
					<td>{{.Title}}{{if .Alias}}<br/><span class="smaller">Som {{.Alias}}</span>{{end}}</td>
					<td>{{range .Forms}}<span class="tag">{{.}}</span> {{end}}</td>
				</tr>
				{{- end}}
			<tbody>
		</table>
		{{end}}
		{{if .Collections}}
		<h3>Samlinger</h3>
		<table class="collections-work-list">
			<tbody>
				{{- range .Collections}}
				<tr>
					<td>{{.FirstPubYear}}</td>
					<td><a href="/{{.ID}}">{{.Title}}</a></td>
					<td>{{range .Forms}}<span class="tag">{{.}}</span> {{end}}</td>
				</tr>
				{{- end}}
			<tbody>
		</table>
		{{end}}
		{{if .Translations}}
		<h3>Oversettelser</h3>
		<table class="translation-work-list">
			<tbody>
				{{- range .Translations}}
				<tr>
					<td>{{.FirstPubYear}}</td>
					<td><a href="/{{.ID}}">{{.Title}}</a>{{if .OriginalTitle}} av {{range .OriginalAuthors}}<a href="/{{.ID}}">{{.Name}}</a> {{end}}<br/><span class="smaller">{{.OriginalTitle}}</span>{{end}}</td>
					<td>{{range .OriginalForms}}<span class="tag">{{.}}</span> {{end}}</td>
				</tr>
				{{- end}}
			<tbody>
		</table>
		{{end}}
		{{end}}
		{{if .WorksAbout}}
		<h2>Litteratur <em>om</em> {{.Name}}</h2>
		<table class="about-work-list">
			<tbody>
				{{- range .WorksAbout}}
				<tr>
					<td>{{.FirstPubYear}}</td>
					<td><a href="/{{.ID}}">{{.Title}}</a>{{if .Authors}} av {{range .Authors}}<a href="/{{.ID}}">{{.Name}}</a>{{end}}{{end}}</td>
					<td>{{range .Forms}}<span class="tag">{{.}}</span> {{end}}</td>
				</tr>
				{{- end}}
			<tbody>
		</table>
		{{end}}
	</main>
</body>
</html>`
)

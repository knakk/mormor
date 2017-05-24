package main

import (
	"html/template"
	"log"
	"net/http"

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
	g, err := e.metadata.triplestore.Describe(rdf.DescForwardRecursive, uri)
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
	Name        string   `rdf:"->hasName"`
	BirthYear   int      `rdf:"->hasBirthYear"`
	DeathYear   int      `rdf:"->hasDeathYear"`
	Description string   `rdf:"->hasDescription;->hasText"`
	Links       []string `rdf:">>hasLink"`
}

const (
	tmplPerson = `
<!doctype html>
<html lang="en">
<head>
	<meta charset=utf-8>
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>{.Title}</title>
	<style>
		*    { box-sizing: border-box }
		html { font-family: Arial,sans-serif; line-height:1.15; -ms-text-size-adjust: 100%; -webkit-text-size-adjust: 100% }
		body { margin: 0; }
		main { max-width: 980px; margin: auto; padding-top: 2em; }

		a:link,
		a:visited { color: blue; }
	</style>
</head>
<body>
	<main>
		<h2>{{.Name}}</h2>
		<p><strong>Født</strong> {{.BirthYear}}</p>
		<p><strong>Død</strong> {{.DeathYear}}</p>
		<p>{{.Description}}</p>

		<h3>Links</h3>
		{{range .Links}}<p><a href="{{.}}">{{.}}</a></p>{{end}}
	</main>
</body>
</html>`
)

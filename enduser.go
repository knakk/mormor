package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/knakk/kbp/rdf"
	"github.com/knakk/kbp/rdf/memory"
	"github.com/knakk/mormor/entity"
)

var (
	langTagToLangs = map[string][]string{
		"no": []string{"lang/nob", "lang/nno"},
		"en": []string{"lang/eng"},
	}
	prefLangs = []string{"lang/nob", "lang/nno", "lang/dan", "lang/swe", "lang/eng", "lang/ger", "lang/fre"}
)

type enduserService struct {
	addr     string
	lang     string
	metadata *metadataService
}

func newEndUserService(addr string, lang string, metadata *metadataService) *enduserService {
	return &enduserService{
		addr:     addr,
		lang:     lang,
		metadata: metadata,
	}
}

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

	if r.URL.Path == "/search" {
		e.serveSearch(w, r)
		return
	}

	if len(r.URL.Path) < 2 {
		http.NotFound(w, r)
		return
	}

	if strings.HasSuffix(r.URL.Path[1:], ".rdf") {
		if err := e.metadata.triplestore.DescribeW(rdf.NewNTriplesEncoder(w), rdf.DescSymmetricRecursive, rdf.NewNamedNode(r.URL.Path[1:len(r.URL.Path)-4])); err != nil {
			log.Printf("%s desribe resource error: %v", r.URL.Path, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		return
	}

	if strings.HasSuffix(r.URL.Path[1:], ".svg") {
		uri := rdf.NewNamedNode(r.URL.Path[1 : len(r.URL.Path)-4])
		g, err := e.metadata.triplestore.Describe(rdf.DescSymmetricRecursive, uri)
		if err != nil {
			log.Printf("%s desribe resource error: %v", r.URL.Path, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		bnodeEdges := map[string][2]string{
			"hasContribution":     [2]string{"hasAgent", "hasRole"},
			"isPublishedInSeries": [2]string{"inSeries", "hasNumber"},
		}

		dot := g.(*memory.Graph).Dot(uri, memory.DotOptions{
			Base:            "",
			Inline:          []string{"hasLink", "hasImage"},
			InlineWithLabel: map[string]string{"hasLiteraryForm": "hasName", "hasLanguage": "hasName", "hasBinding": "hasName"},
			FullTypes:       []string{"Person", "Corporation", "Publication", "Place", "Alias", "Work", "Event", "PublisherSeries"},
			BnodeEdges:      bnodeEdges})
		cmd := exec.Command("dot", "-Tsvg")
		cmd.Stdin = strings.NewReader(dot)
		cmd.Stdout = w
		cmd.Stderr = os.Stdout
		if err := cmd.Run(); err != nil {
			log.Println(err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		return
	}

	paths := strings.Split(r.URL.Path[1:], "/")
	if len(paths) < 2 {
		http.NotFound(w, r)
		return
	}

	switch paths[0] {
	case "person":
		e.servePerson(w, r, strings.Join(paths, "/"))
	case "work":
		if len(paths) != 4 {
			http.NotFound(w, r)
			return
		}
		e.servePublication(w, r, strings.Join(paths[:2], "/"), strings.Join(paths[2:], "/"))
	case "publisherSeries":
		e.servePublisherSeries(w, r, strings.Join(paths, "/"))
	case "static":
		http.ServeFile(w, r, strings.Join(paths, "/"))
	default:
		http.NotFound(w, r)
	}
}

func (e *enduserService) serveSearch(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Query()["q"]) != 1 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	q := r.URL.Query()["q"][0]
	res, err := e.metadata.searchService.queryAll(q)
	if err != nil {
		log.Printf("search query error %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(res)
}

func (e *enduserService) servePerson(w http.ResponseWriter, r *http.Request, personID string) {
	g, err := e.metadata.triplestore.Describe(rdf.DescSymmetricRecursive, rdf.NewNamedNode(personID))
	if err != nil {
		log.Printf("%s desribe resource error: %v", r.URL.Path, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	var p entity.Person
	if err := g.(*memory.Graph).Decode(&p, rdf.NewNamedNode(personID), rdf.NewNamedNode(""), []string{e.lang}); err != nil {
		log.Printf("%s decode Person error: %v", r.URL.Path, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	p.Process()

	/*
		// Order translations by language score
		for wi := range p.Works {
			sort.Slice(p.Works[wi].Translations, func(i, j int) bool {
				a, b := 99, 99
				for ii, l := range prefLangs {
					if l == p.Works[wi].Translations[i].Language.URI {
						a = ii
						break
					}
				}
				for jj, l := range prefLangs {
					if l == p.Works[wi].Translations[j].Language.URI {
						b = jj
						break
					}
				}
				return a < b
			})
		}
	*/
	//spew.Dump(p)
	//fmt.Printf("%+v\n", p)

	if err := templates.ExecuteTemplate(w, "person.html", struct {
		Person *entity.Person
		Langs  []string
	}{
		Person: &p,
		Langs:  langTagToLangs[e.lang],
	}); err != nil {
		log.Printf("%s: %v", r.URL.Path, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (e *enduserService) servePublication(w http.ResponseWriter, r *http.Request, workID, pubID string) {
	g, err := e.metadata.triplestore.Describe(rdf.DescSymmetricRecursive, rdf.NewNamedNode(workID))
	if err != nil {
		log.Printf("%s desribe resource error: %v", r.URL.Path, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	var wrk entity.WorkWithPublications
	if err := g.(*memory.Graph).Decode(&wrk, rdf.NewNamedNode(workID), rdf.NewNamedNode(""), []string{e.lang}); err != nil {
		log.Printf("%s: %v", r.URL.Path, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	var selected entity.Publication
	for i, p := range wrk.Publications {
		if p.URI == pubID {
			selected = p
			wrk.Publications = append(wrk.Publications[:i], wrk.Publications[i+1:]...)
			break
		}
	}
	if selected.URI == "" {
		http.NotFound(w, r)
		return
	}
	if err := templates.ExecuteTemplate(w, "work.html", struct {
		Selected entity.Publication
		Work     *entity.WorkWithPublications
	}{Selected: selected, Work: &wrk}); err != nil {
		log.Printf("%s: %v", r.URL.Path, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (e *enduserService) servePublisherSeries(w http.ResponseWriter, r *http.Request, seriesID string) {
	g, err := e.metadata.triplestore.Describe(rdf.DescSymmetricRecursive, rdf.NewNamedNode(seriesID))
	if err != nil {
		log.Printf("%s desribe resource error: %v", r.URL.Path, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	var s entity.PublisherSeries
	if err := g.(*memory.Graph).Decode(&s, rdf.NewNamedNode(seriesID), rdf.NewNamedNode(""), []string{e.lang}); err != nil {
		log.Printf("%s decode PublisherSeries error: %v", r.URL.Path, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	s.Process()
	//spew.Dump(s)
	//fmt.Printf("%+v\n", p)

	if err := templates.ExecuteTemplate(w, "publisher_series.html", &s); err != nil {
		log.Printf("%s: %v", r.URL.Path, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

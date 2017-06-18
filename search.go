package main

import (
	"fmt"

	"github.com/blevesearch/bleve"
	"github.com/knakk/kbp/rdf"
	"github.com/knakk/kbp/rdf/memory"
	"github.com/knakk/mormor/entity"
)

// doc is a document representing a resource that can be indexed and retrieved
type doc struct {
	Title    string
	ID       string
	Abstract string
	Type     string
}

type searchResults struct {
	NumHits int
	Hits    []doc
}

type searchService struct {
	Index bleve.Index
}

func newSearchService() *searchService {
	index, err := bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		panic(err)
	}
	return &searchService{
		Index: index,
	}
}

func (s *searchService) indexResourceFromGraph(uri rdf.NamedNode, g *memory.Graph) error {
	var e entity.Entity
	switch entity.TypeFromURI(uri) {
	case entity.TypePerson:
		var p entity.Person
		if err := g.Decode(&p, uri, rdf.NewNamedNode("")); err != nil {
			return fmt.Errorf("indexResourceFromGraph decode %s as Person error: %v", uri, err)
		}
		p.Process()
		e = &p
	/*case entity.TypePublication:
	var p entity.PublicationWithWork
	if err := g.Decode(&p, uri, rdf.NewNamedNode("")); err != nil {
		return fmt.Errorf("indexResourceFromGraph decode %s as Publication error: %v", uri, err)
	}
	e = p
	*/
	case entity.TypeWork:
		var w entity.Work
		if err := g.Decode(&w, uri, rdf.NewNamedNode("")); err != nil {
			return fmt.Errorf("indexResourceFromGraph decode %s as Work error: %v", uri, err)
		}
		w.Process()
		e = &w
	default:
		panic("TODO indexResourceFromGraph " + entity.TypeFromURI(uri).String())
	}

	d := doc{
		Title:    e.CanonicalTitle(),
		Abstract: e.Abstract(),
		ID:       e.ID(),
		Type:     e.EntityType().String(),
	}
	return s.Index.Index(uri.Name(), d)
}

func (s *searchService) query(idx entity.Type, q string) (searchResults, error) {
	query := bleve.NewQueryStringQuery("+Type:" + idx.String() + " +" + q)
	req := bleve.NewSearchRequest(query)
	res, err := s.Index.Search(req)
	if err != nil {
		return searchResults{}, err
	}
	return s.parseSearchResults(res)
}

func (s *searchService) queryAll(q string) (searchResults, error) {
	query := bleve.NewQueryStringQuery(q)
	req := bleve.NewSearchRequest(query)
	res, err := s.Index.Search(req)
	if err != nil {
		return searchResults{}, err
	}
	return s.parseSearchResults(res)
}

func (s *searchService) parseSearchResults(res *bleve.SearchResult) (searchResults, error) {
	parsed := searchResults{
		NumHits: int(res.Total),
		Hits:    make([]doc, 0, res.Hits.Len()),
	}
	for _, hit := range res.Hits {
		stored, err := s.Index.Document(hit.ID)
		if err != nil {
			return parsed, err
		}
		var d doc
		for _, field := range stored.Fields {
			switch field.Name() {
			case "ID":
				d.ID = string(field.Value())
			case "Title":
				d.Title = string(field.Value())
			case "Abstract":
				d.Abstract = string(field.Value())
			case "Type":
				d.Type = string(field.Value())
			}
		}
		parsed.Hits = append(parsed.Hits, d)
	}
	return parsed, nil
}

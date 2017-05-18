package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/knakk/kbp/rdf"
	"github.com/knakk/kbp/rdf/disk"
)

type metadataService struct {
	addr        string
	dbPath      string
	ns          string
	triplestore rdf.Graph
}

func newMetadataService(addr, dbPath, ns string) *metadataService {
	return &metadataService{
		addr:   addr,
		dbPath: dbPath,
		ns:     ns,
	}
}

func (m *metadataService) String() string { return "metadata" }

func (m *metadataService) Start() error {
	db, err := disk.Open(m.dbPath, m.ns)
	if err != nil {
		return err
	}
	m.triplestore = db

	log.Printf("starting metadata service listening at %s", m.addr)
	return http.ListenAndServe(m.addr, m)
}

func (m *metadataService) Stop() error {

	if g, ok := m.triplestore.(*disk.Graph); ok {
		return g.Close()
	}

	return nil
}

func (m *metadataService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/resource/") {
		http.NotFound(w, r)
		return
	}

	resources := strings.Split(strings.TrimPrefix(r.URL.Path, "/resource/"), "+")
	for i := range resources {
		resources[i] = strings.TrimPrefix(resources[i], "/")
	}

	if len(resources) == 0 {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case "GET":
		nodes := make([]rdf.NamedNode, len(resources))
		for i, r := range resources {
			nodes[i] = rdf.NewNamedNode(m.ns + r)
		}
		w.Header().Set("Content-Type", "application/n-triples")
		if err := m.triplestore.DescribeW(rdf.NewNTriplesEncoder(w), nodes...); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	case "PATCH":
		defer r.Body.Close()
		q, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("%s update read request error: %v", r.URL.Path, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		del, ins, where, err := rdf.ParseUpdateQuery(string(q))
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		if outOfBoundsQ(resources, del, ins, where) {
			http.Error(w, "bad request: trying to update unrelated resources", http.StatusBadRequest)
			return
		}

		nd, ni, err := m.triplestore.Update(del, ins, where)
		if err != nil {
			log.Printf("%s update query error: %v", r.URL.Path, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		log.Printf("%s update OK: deleted: %d; inserted: %d", r.URL.Path, nd, ni)
		//fmt.Fprintf(w, "OK: deleted: %d; inserted: %d", nd, ni)
	//case "POST":
	//case "DELETE":
	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
}

func outOfBoundsQ(resources []string, del, ins, where []rdf.TriplePattern) bool {
	return subjectInResources(resources, del) &&
		subjectInResources(resources, ins) &&
		subjectInResources(resources, where)
}

func subjectInResources(resources []string, patterns []rdf.TriplePattern) bool {
	for _, candidate := range patterns {
		if node, ok := candidate.Subject.(rdf.NamedNode); ok {
			// Only allowed if node is in resources.
			if inResources(node.Name(), resources) {
				return false
			}
		}
	}
	return true
}

func inResources(find string, resources []string) bool {
	for _, r := range resources {
		if r == find {
			return true
		}
	}
	return false
}

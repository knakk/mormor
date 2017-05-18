package main

import (
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
			http.Error(w,
				http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError)

			return
		}
	//case "POST":
	//case "PATCH":
	//case "DELETE":
	default:
		http.Error(w,
			http.StatusText(http.StatusMethodNotAllowed),
			http.StatusMethodNotAllowed)

		return
	}
}

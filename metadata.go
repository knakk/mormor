package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/knakk/kbp/rdf"
	"github.com/knakk/kbp/rdf/disk"
	"github.com/knakk/kbp/rdf/memory"
)

type metadataService struct {
	addr          string
	dbPath        string
	ns            string
	triplestore   rdf.Graph
	searchService *searchService
	indexingQueue chan rdf.NamedNode
	idcount       int32
	//ontology  rdf.Ontology
}

func newMetadataService(addr, dbPath, ns string) *metadataService {
	m := metadataService{
		addr:          addr,
		dbPath:        dbPath,
		ns:            ns,
		indexingQueue: make(chan rdf.NamedNode),
	}
	go m.processIndexingQueue()
	return &m
}

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
	log.Println("shutting down metadata service")

	if g, ok := m.triplestore.(*disk.Graph); ok {
		return g.Close()
	}

	return nil
}

// base32 encoding alphabet
const base32 = "0123456789abcdefghjkmnpqrstvwxyz"

// nextID generates a ID that can be used for generating an unique URI for a
// RDF resource. The ID is comprised of a 10-byte timestamp with millisecond
// resolution, and 2 bytes from an incremental number, which means  you can
// create 32^2 resources per millisecond without risking any collisions,
// which is most likely much more than we can process anyway.
func (m *metadataService) nextID(resType string) string {

	// Base32-encoding of timestamp taken from github.com/oklog/ulid

	// Get current time in Unix milliseconds:
	now := time.Now().UTC()
	ms := uint64(now.Unix())*1000 + uint64(now.Nanosecond()/int(time.Millisecond))

	id := make([]byte, 6)
	id[0] = byte(ms >> 40)
	id[1] = byte(ms >> 32)
	id[2] = byte(ms >> 24)
	id[3] = byte(ms >> 16)
	id[4] = byte(ms >> 8)
	id[5] = byte(ms)

	dst := make([]byte, 13)

	dst[0] = base32[(id[0]&224)>>5]
	dst[1] = base32[id[0]&31]
	dst[2] = base32[(id[1]&248)>>3]
	dst[3] = base32[((id[1]&7)<<2)|((id[2]&192)>>6)]
	dst[4] = base32[(id[2]&62)>>1]
	dst[5] = base32[((id[2]&1)<<4)|((id[3]&240)>>4)]
	dst[6] = base32[((id[3]&15)<<1)|((id[4]&128)>>7)]
	dst[7] = base32[(id[4]&124)>>2]
	dst[8] = base32[((id[4]&3)<<3)|((id[5]&224)>>5)]
	dst[9] = base32[id[5]&31]

	n := atomic.AddInt32(&m.idcount, 1) % 1024
	//dst[10] = base32[byte(((n>>16)&124)>>3)]
	dst[10] = base32[byte(((n>>8)&3<<3)|(n&224>>5))]
	dst[11] = base32[byte(n)&31]

	return string(dst)
}

func (m *metadataService) indexOnly(uri rdf.NamedNode) {
	go func() {
		m.indexingQueue <- uri
	}()
}

func (m *metadataService) processIndexingQueue() {
	for uri := range m.indexingQueue {
		g, err := m.triplestore.Describe(rdf.DescSymmetricRecursive, uri)
		if err != nil {
			log.Printf("desribe resource %v error: %v", uri, err)
			continue
		}
		if err := m.searchService.indexResourceFromGraph(uri, g.(*memory.Graph)); err != nil {
			log.Printf("indexing %v error: %v", uri, err)
			continue
		}
		log.Println("indexed " + uri.String())
	}
}

func (m *metadataService) CreateResource(resType string, body io.Reader) (rdf.NamedNode, error) {
	uri := rdf.NewNamedNode(m.ns + resType + "/" + m.nextID(resType))
	trs := []rdf.Triple{
		rdf.Triple{
			Subject:   uri,
			Predicate: rdf.RDFtype,
			Object:    rdf.NewNamedNode(strings.Title(resType)),
		},
	}
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return uri, err
	}
	if len(b) > 0 {
		b = bytes.Replace(b, []byte("<>"), []byte(uri.String()), -1)
		dec := rdf.NewDecoder(bytes.NewBuffer(b))
		for tr, err := dec.Decode(); err != io.EOF; tr, err = dec.Decode() {
			if err != nil {
				return uri, err
			}
			trs = append(trs, tr)
		}
	}

	_, err = m.triplestore.Insert(trs...)

	if err == nil {
		// Since it's a new resource, we can safely assume it has no connected resource we also need to reindex.
		m.indexOnly(uri)
	}

	return uri, err
}

func (m *metadataService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
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
		//w.Header().Set("Content-Type", "application/n-triples")
		if err := m.triplestore.DescribeW(rdf.NewNTriplesEncoder(w), rdf.DescForward, nodes...); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	case "PATCH":
		q, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("%s update read request error: %v", r.URL.Path, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		del, ins, where, err := rdf.ParseUpdateQuery(string(q))
		if err != nil {
			http.Error(w, "bad request: error in update query: "+err.Error(), http.StatusBadRequest)
			return
		}

		if outOfBoundsQuery(resources, del, ins, where) {
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
	case "POST":
		if len(resources) > 1 {
			http.Error(w, "bad request: can only create one resource at a time", http.StatusBadRequest)
			return
		}
		uri, err := m.CreateResource(resources[0], r.Body)
		if err != nil {
			log.Printf("%s create resource error: %v", r.URL.Path, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Location", uri.Name())
		w.WriteHeader(http.StatusCreated)
	//case "DELETE":
	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
}

// outOfBoundsQuery tests if the query would remove or add triples to resources not in
// the specified resources list.
func outOfBoundsQuery(resources []string, del, ins, where []rdf.TriplePattern) bool {
	return subjectNotInResources(resources, del, where) ||
		subjectNotInResources(resources, ins, where)
}

func subjectNotInResources(resources []string, op, where []rdf.TriplePattern) bool {
	if len(op) == 0 {
		return false
	}
	for _, candidate := range op {
		if node, ok := candidate.Subject.(rdf.NamedNode); ok {
			// Only allowed if node is in resources.
			if inResources(node.Name(), resources) {
				return false
			}
		} else if vs, ok := candidate.Subject.(rdf.Variable); ok {
			// Only allowed if node is object in triple where subject
			// node is in resources
			for _, p := range where {
				if vo, ok := p.Object.(rdf.Variable); ok && vs == vo {
					if node, ok := p.Subject.(rdf.NamedNode); ok && inResources(node.Name(), resources) {
						return false
					}
				}
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

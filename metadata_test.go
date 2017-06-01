package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/knakk/kbp/rdf"
	"github.com/knakk/kbp/rdf/memory"
	"github.com/knakk/mormor/entity"
)

func decodeGraph(d *rdf.Decoder) (*memory.Graph, error) {
	g := memory.NewGraph()
	bnodeTriples := make(map[rdf.BlankNode][]rdf.Triple)

	for tr, err := d.Decode(); err != io.EOF; tr, err = d.Decode() {
		if err != nil {
			return g, err
		}
		switch t := tr.Subject.(type) {
		case rdf.BlankNode:
			bnodeTriples[t] = append(bnodeTriples[t], tr)
			continue
		}
		switch t := tr.Object.(type) {
		case rdf.BlankNode:
			bnodeTriples[t] = append(bnodeTriples[t], tr)
			continue
		}
		if _, err := g.Insert(tr); err != nil {
			return nil, err
		}
	}

	// Insert triples with bnodes in batches by ID, so that they get assigned
	// the same (new) blank node ID in the Graph
	for _, trs := range bnodeTriples {
		if _, err := g.Insert(trs...); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func mustDecode(s string) *memory.Graph {
	dec := rdf.NewDecoder(bytes.NewBufferString(s))
	g, err := decodeGraph(dec)
	if err != nil {
		panic("mustDecode: " + err.Error())
	}
	return g
}

func mustEncode(g *memory.Graph) string {
	var b bytes.Buffer
	if err := g.EncodeNTriples(&b); err != nil {
		panic(err)
	}
	return b.String()
}

func testWantGraph(t *testing.T, method string, url string, body string, wantGraph string) {
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("got %v; want 200 OK", resp.Status)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got := mustDecode(string(b))
	want := mustDecode(wantGraph)
	if !got.Eq(want) {
		t.Fatalf("got:\n%v\nwant:\n%v", mustEncode(got), mustEncode(want))
	}
}

func testWantStatus(t *testing.T, method, url, body string, status int) *http.Response {
	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != status {
		t.Fatalf("got %v; want %v", resp.Status, status)
	}
	return resp
}

func newTestSearchService() *searchService {
	index, err := bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		panic(err)
	}
	return &searchService{
		Index: index,
	}
}

func testWantSearchResultsToContain(t *testing.T, s *searchService, idx entity.Type, q string, want ...doc) {

	timeout := time.After(1 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond).C

	// Keep trying to query every tick until timeout:
	for {
		select {
		case <-timeout:
			t.Fatalf("docs not found in index after 1 second: %+v", want)
		case <-tick:
			res, err := s.query(idx, q)
			if err != nil {
				t.Fatal(err)
			}
			foundN := 0
			for _, wantDoc := range want {
				for _, gotDoc := range res.Hits {
					if wantDoc == gotDoc {
						foundN++
					}
				}
			}
			if foundN == len(want) {
				return
			}
		}
	}
}

// Verify that resources can be created, fetched, updated and deleted (TODO),
// and that the resources are indexed reflecting any changes.
func TestResourceLifecycle(t *testing.T) {
	m := &metadataService{
		triplestore:   memory.NewGraph(),
		searchService: newTestSearchService(),
		indexingQueue: make(chan rdf.NamedNode),
	}
	go m.processIndexingQueue()
	srv := httptest.NewServer(m)

	// Create two resources and find their URIs in Location header
	resp := testWantStatus(t, "POST", srv.URL+"/resource/person",
		`<> <hasName> "Name" .
		 <> <hasBirthYear> "1988"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
		http.StatusCreated)
	personuri := resp.Header.Get("Location")
	if personuri == "" {
		t.Fatal("URI of created resource not found in Location header")
	}

	resp = testWantStatus(t, "POST", srv.URL+"/resource/work",
		`<> <hasMainTitle> "title" .
		 <> <hasContribution> _:c .
		 _:c <hasAgent> <`+personuri+`> .
		 _:c <hasRole> <author> .`,
		http.StatusCreated)
	bookuri := resp.Header.Get("Location")
	if bookuri == "" {
		t.Fatal("URI of created resource not found in Location header")
	}

	rpl := strings.NewReplacer("bookuri", bookuri, "personuri", personuri)

	// Fetch single resource
	testWantGraph(t, "GET", srv.URL+"/resource/"+personuri, "",
		rpl.Replace(`<personuri> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <Person> .
					 <personuri> <hasName> "Name" .
					 <personuri> <hasBirthYear> "1988"^^<http://www.w3.org/2001/XMLSchema#integer> .`))

	// Fetch multiple resouces
	testWantGraph(t, "GET", srv.URL+"/resource/"+personuri+"+"+bookuri, "",
		rpl.Replace(`<personuri> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <Person> .
					 <personuri> <hasName> "Name" .
					 <personuri> <hasBirthYear> "1988"^^<http://www.w3.org/2001/XMLSchema#integer> .
					 <bookuri> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <Work> .
					 <bookuri> <hasMainTitle> "title" .
					 <bookuri> <hasContribution> _:c .
					 _:c <hasAgent> <personuri> .
					 _:c <hasRole> <author> .`))

	// Find resources in index
	testWantSearchResultsToContain(t, m.searchService, entity.TypePerson, "Name",
		doc{Title: "Name (1988-)", ID: personuri, Type: "Person"})

	testWantSearchResultsToContain(t, m.searchService, entity.TypeWork, "Name",
		doc{Title: "Name: title", ID: bookuri, Type: "Work"})

	// Update resource
	testWantStatus(t, "PATCH", srv.URL+"/resource/"+personuri,
		rpl.Replace(`- <personuri> <hasBirthYear> "1988"^^<http://www.w3.org/2001/XMLSchema#integer> .
					 + <personuri> <hasBirthYear> "1888"^^<http://www.w3.org/2001/XMLSchema#integer> .
					 + <personuri> <hasDeathYear> "1958"^^<http://www.w3.org/2001/XMLSchema#integer> .`),
		http.StatusOK)

	// Verify it's been updated and indexed
	testWantGraph(t, "GET", srv.URL+"/resource/"+personuri, "",
		rpl.Replace(`<personuri> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <Person> .
					 <personuri> <hasName> "Name" .
					 <personuri> <hasBirthYear> "1888"^^<http://www.w3.org/2001/XMLSchema#integer> .
					 <personuri> <hasDeathYear> "1958"^^<http://www.w3.org/2001/XMLSchema#integer> .`))

	testWantSearchResultsToContain(t, m.searchService, entity.TypePerson, "Name",
		doc{Title: "Name (1888-1958)", ID: personuri, Type: "Person"})

	// Update bnode resource
	testWantStatus(t, "PATCH", srv.URL+"/resource/"+bookuri,
		rpl.Replace(`- ?c <hasRole> <author> .
					 + ?c <hasRole> <editor> .
					 <bookuri> <hasContribution> ?c .
					 ?c <hasAgent> <personuri> .
					 ?c <hasRole> <author> .`),
		http.StatusOK)

	// Verify it's been updated
	testWantGraph(t, "GET", srv.URL+"/resource/"+bookuri, "",
		rpl.Replace(`<bookuri> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <Work> .
					 <bookuri> <hasMainTitle> "title" .
					 <bookuri> <hasContribution> _:c .
					 _:c <hasAgent> <personuri> .
					 _:c <hasRole> <editor> .`))

	// Verify you cannot update resources not "in focus"
	testWantStatus(t, "PATCH", srv.URL+"/resource/book1",
		rpl.Replace(`- ?c <hasRole> <author> .
					 + ?c <hasRole> <editor> .
					 <book123> <hasContribution> ?c .
					 ?c <hasAgent> <personuri> .
					 ?c <hasRole> <author> .`),
		http.StatusBadRequest)
}

// Verify that resources are indexed, kept up to date on changes, and can be retrieved.
func TestIndexingAndSearchingResources(t *testing.T) {
	t.Skip()
	m := &metadataService{
		triplestore:   memory.NewGraph(),
		searchService: newTestSearchService(),
		indexingQueue: make(chan rdf.NamedNode),
	}
	go m.processIndexingQueue()

	srv := httptest.NewServer(m)

	// Create resource and find it
	resp := testWantStatus(t, "POST", srv.URL+"/resource/person",
		`<> <hasName> "Kari Olsen" .`,
		http.StatusCreated)
	kariID := resp.Header.Get("Location")
	if kariID == "" {
		t.Fatal("URI of created resource not found in Location header")
	}

	testWantSearchResultsToContain(t, m.searchService, entity.TypePerson, "Olsen",
		doc{Title: "Kari Olsen", ID: kariID})

	// Create another resource and find both
	resp = testWantStatus(t, "POST", srv.URL+"/resource/person",
		`<> <hasName> "Knut Olsen" .`,
		http.StatusCreated)
	knutID := resp.Header.Get("Location")
	if knutID == "" {
		t.Fatal("URI of created resource not found in Location header")
	}

	testWantSearchResultsToContain(t, m.searchService, entity.TypePerson, "Olsen",
		doc{Title: "Kari Olsen", ID: kariID},
		doc{Title: "Knut Olsen", ID: knutID})

	// Update resource and verify the indexed version get's uptdated too
	testWantStatus(t, "PATCH", srv.URL+"/resource/"+kariID,
		fmt.Sprintf(`
			- <%s> <hasName> "Kari Olsen" .
			+ <%s> <hasName> "Kari Knutsdatter Olsen" .`, kariID, kariID),
		http.StatusOK)
}

func TestOutOfBoundsQ(t *testing.T) {
	tests := []struct {
		query     string
		resources []string
		want      bool
	}{
		{ // 1
			`- <book> ?p ?o .
			<book> ?p ?o .`,
			[]string{"book"},
			false,
		},
		{ // 2
			`- <book> ?p ?o .
			<book> ?p ?o .`,
			[]string{"books", "book1", "book2"},
			true,
		},
		{ // 3
			`- ?book ?p ?o .
			<person> <wrote> ?book .
			?book ?p ?o .`,
			[]string{"person"},
			false,
		},
		{ // 4
			`+ ?book <title> "xyz" .
			<person> <wrote> ?book .
			?book ?p ?o .`,
			[]string{"book", "person1"},
			true,
		},
		{ // 5
			`- ?s ?p ?o .
			?s ?p ?o .`,
			[]string{"book/123", "person/456"},
			true,
		},
		{ // 6
			`- ?work ?p ?o .
			<person> <created> ?book .
			?book <isPublicationOf> ?work .
			?work ?p ?o .`,
			[]string{"person"},
			true,
		},
		{ // 7
			`- ?work ?p ?o .
			<person> <created> ?book .
			?book <isPublicationOf> <work> .
			?work ?p ?o .`,
			[]string{"person", "work"},
			true,
		},
		{ // 8
			`- ?work <title> "x" .
			 + ?work <title> "y" .
			<person> <created> ?work .
			?work <title> "x" .`,
			[]string{"person"},
			false,
		},
		{ // 9
			`- ?work <title> "x" .
			 + ?work <title> "y" .
			<person> <created> ?work .
			?work <title> "x" .`,
			[]string{"work"},
			true,
		},
	}

	for i, test := range tests {
		del, ins, where, err := rdf.ParseUpdateQuery(test.query)
		if err != nil {
			t.Fatal(err)
		}
		if got := outOfBoundsQuery(test.resources, del, ins, where); got != test.want {
			t.Errorf("outOfBoundsQuery #%d: got %v; want %v", i+1, got, test.want)
		}
	}
}

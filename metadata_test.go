package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/knakk/kbp/rdf"
	"github.com/knakk/kbp/rdf/memory"
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

// Verify that resources can be created, fetched and updated.
func TestCreateGetAndUpdateResources(t *testing.T) {
	m := &metadataService{
		triplestore: memory.NewGraph(),
	}

	srv := httptest.NewServer(m)

	// Create two resources and find their URIs in Location header
	resp := testWantStatus(t, "POST", srv.URL+"/resource/person",
		`<> <hasName> "Name" .
		 <> <hasBirthYear> "1988" .`,
		http.StatusCreated)
	personuri := resp.Header.Get("Location")
	if personuri == "" {
		t.Fatal("URI of created resource not found in Location header")
	}

	resp = testWantStatus(t, "POST", srv.URL+"/resource/book",
		`<> <hasTitle> "title" .
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
		 			 <personuri> <hasBirthYear> "1988" .`))

	// Fetch multiple resouces
	testWantGraph(t, "GET", srv.URL+"/resource/"+personuri+"+"+bookuri, "",
		rpl.Replace(`<personuri> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <Person> .
					 <personuri> <hasName> "Name" .
					 <personuri> <hasBirthYear> "1988" .
					 <bookuri> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <Book> .
					 <bookuri> <hasTitle> "title" .
					 <bookuri> <hasContribution> _:c .
					 _:c <hasAgent> <personuri> .
					 _:c <hasRole> <author> .`))

	// Update resource
	testWantStatus(t, "PATCH", srv.URL+"/resource/"+personuri,
		rpl.Replace(`- <personuri> <hasBirthYear> "1988" .
					 + <personuri> <hasBirthYear> "1888" .
					 + <personuri> <hasDeathYear> "1958" .`),
		http.StatusOK)

	// Verify it's been updated
	testWantGraph(t, "GET", srv.URL+"/resource/"+personuri, "",
		rpl.Replace(`<personuri> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <Person> .
					 <personuri> <hasName> "Name" .
					 <personuri> <hasBirthYear> "1888" .
					 <personuri> <hasDeathYear> "1958" .`))

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
		rpl.Replace(`<bookuri> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <Book> .
					 <bookuri> <hasTitle> "title" .
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

package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
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

// Verify that resources can be fetched.
func TestGetResources(t *testing.T) {
	m := &metadataService{
		triplestore: mustDecode(
			`<person> <hasName> "Name" .
			 <person> <hasBirthYear> "1988" .
			 <book> <hasTitle> "title" .
			 <book> <hasContribution> _:c .
			 _:c <hasAgent> <person> .
			 _:c <hasRole> <author> .`,
		),
	}

	srv := httptest.NewServer(m)

	// Fetch single resource
	resp, err := http.Get(srv.URL + "/resource/person")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	got := mustDecode(string(b))
	want := mustDecode(
		`<person> <hasName> "Name" .
		 <person> <hasBirthYear> "1988" .`)
	if !got.Eq(want) {
		t.Errorf("got:\n%v\nwant:\n%v", mustEncode(got), mustEncode(want))
	}

	// Fetch multiple resouces
	resp, err = http.Get(srv.URL + "/resource/person+book")
	if err != nil {
		t.Fatal(err)
	}
	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	got = mustDecode(string(b))
	want = mustDecode(
		`<person> <hasName> "Name" .
		 <person> <hasBirthYear> "1988" .
		 <book> <hasTitle> "title" .
		 <book> <hasContribution> _:c .
		 _:c <hasAgent> <person> .
		 _:c <hasRole> <author> .`)
	if !got.Eq(want) {
		t.Errorf("got:\n%v\nwant:\n%v", mustEncode(got), mustEncode(want))
	}

}

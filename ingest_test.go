package main

import (
	"bytes"
	"testing"

	"github.com/knakk/kbp/rdf"
)

func TestIngestOriaMetadata(t *testing.T) {
	// https://bibsys.alma.exlibrisgroup.com/view/sru/47BIBSYS_NETWORK?version=1.2&operation=searchRetrieve&recordSchema=marcxml&query=alma.isbn=0804810346
	const input = `<?xml version="1.0" encoding="UTF-8"?><searchRetrieveResponse xmlns="http://www.loc.gov/zing/srw/">
  <version>1.2</version>
  <numberOfRecords>1</numberOfRecords>
  <records>
    <record>
      <recordSchema>marcxml</recordSchema>
      <recordPacking>xml</recordPacking>
      <recordData>
        <record xmlns="">
          <leader>00715cam a2200241 c 4500</leader>
          <controlfield tag="001">990114007574702201</controlfield>
          <controlfield tag="005">20150326113052.0</controlfield>
          <controlfield tag="007">ta</controlfield>
          <controlfield tag="008">150326s1972    xx#|||||||||||000|u|eng|d</controlfield>
          <datafield tag="020" ind1=" " ind2=" ">
            <subfield code="a">0804810346</subfield>
            <subfield code="q">ib.</subfield>
          </datafield>
          <datafield tag="035" ind1=" " ind2=" ">
            <subfield code="a">011400757-47bibsys_network</subfield>
          </datafield>
          <datafield tag="035" ind1=" " ind2=" ">
            <subfield code="a">(NO-TrBIB)011400757</subfield>
          </datafield>
          <datafield tag="040" ind1=" " ind2=" ">
            <subfield code="a">NO-TrBIB</subfield>
            <subfield code="b">nob</subfield>
            <subfield code="e">katreg</subfield>
          </datafield>
          <datafield tag="100" ind1="1" ind2=" ">
            <subfield code="a">Natsume, Sôseki</subfield>
            <subfield code="d">1867-1916</subfield>
            <subfield code="0">(NO-TrBIB)98075025</subfield>
          </datafield>
          <datafield tag="245" ind1="1" ind2="0">
            <subfield code="a">I am a cat</subfield>
            <subfield code="c">Sôseki Natsume</subfield>
          </datafield>
          <datafield tag="246" ind1="1" ind2=" ">
            <subfield code="a">Wagahai wa neko de aru</subfield>
            <subfield code="i">Originaltittel</subfield>
          </datafield>
          <datafield tag="260" ind1=" " ind2=" ">
            <subfield code="a">Rutland, VT</subfield>
            <subfield code="b">Tuttle</subfield>
            <subfield code="c">1972</subfield>
          </datafield>
          <datafield tag="300" ind1=" " ind2=" ">
            <subfield code="a">218 s.</subfield>
          </datafield>
          <datafield tag="500" ind1=" " ind2=" ">
            <subfield code="a">Translated by Aiko Ito &amp; Graeme Wilson</subfield>
          </datafield>
          <datafield tag="653" ind1=" " ind2=" ">
            <subfield code="a">japansk</subfield>
            <subfield code="a">litteratur</subfield>
            <subfield code="a">roman</subfield>
          </datafield>
          <datafield tag="700" ind1="1" ind2=" ">
            <subfield code="a">Itō, Aiko</subfield>
            <subfield code="0">(NO-TrBIB)90764137</subfield>
          </datafield>
          <datafield tag="700" ind1="1" ind2=" ">
            <subfield code="a">Wilson, Graeme</subfield>
            <subfield code="0">(NO-TrBIB)90764138</subfield>
          </datafield>
          <datafield tag="852" ind1="0" ind2="1">
            <subfield code="a">47BIBSYS_UBO</subfield>
            <subfield code="6">990114007574702204</subfield>
            <subfield code="9">P</subfield>
          </datafield>
          <datafield tag="852" ind1="0" ind2="1">
            <subfield code="a">47BIBSYS_HIO</subfield>
            <subfield code="6">990114007574702218</subfield>
            <subfield code="9">P</subfield>
          </datafield>
          <datafield tag="901" ind1=" " ind2=" ">
            <subfield code="a">80</subfield>
          </datafield>
        </record>
      </recordData>
      <recordPosition>1</recordPosition>
    </record>
  </records>
  <diagnostics/>
  <extraResponseData xmlns:xb="http://www.exlibris.com/repository/search/xmlbeans/">
    <xb:exact>true</xb:exact>
    <xb:responseDate>2017-07-23T19:50:02+0200</xb:responseDate>
  </extraResponseData>
</searchRetrieveResponse>`

	const want = `
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .

<p> a <Publication> ;
	<hasISBN> "0804810346" ;
	<hasMainTitle> "I am a cat" ;
	<hasPublishYear> "1972"^^xsd:int ;
	<hasPublisher> [
		a <Publisher> ;
		<hasName> "Tuttle"
	] ;
	<hasBinding> <binding/hardback> ;
	<hasNumPages> "218"^^xsd:int ;
	<isPublicationOf> [
		a <Work> ;
		<hasName> "I am a cat"@eng ;
		<hasLanguage> <lang/eng> ;
		<hasLiteraryForm> <form/novel> ;
		<isTranslationOf> [
			a <Work> ;
			<hasName> "Wagahai wa neko de aru" ;
			<hasContribution> [
				a <Contribution> ;
				<hasRole> <role/author> ;
				<hasAgent> [
					a <Person> ;
					<hasName> "Sôseki Natsume" ;
					<hasBirthDate> [
						a <Date> ;
						<hasYear> "1867"^^xsd:int
					] ;
					<hasDeathDate> [
						a <Date> ;
						<hasYear> "1916"^^xsd:int
					]
				]
			]
		] ;
		<hasContribution> [
			a <Contribution> ;
			<hasAgent> [
				a <Person> ;
				<hasName> "Aiko Itō"
			]
		] ;
		<hasContribution> [
			a <Contribution> ;
			<hasAgent> [
				a <Person> ;
				<hasName> "Graeme Wilson"
			]
		]
	] .
	`

	got, err := ingestPublication(rdf.NewNamedNode("p"), bytes.NewBufferString(input), sourceOria)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Eq(mustDecode(want)) {
		t.Errorf("got:\n%v\nwant:\n%v", mustEncode(got), mustEncode(mustDecode(want)))
	}
}

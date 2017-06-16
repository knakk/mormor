package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// globals
var (
	templates = template.Must(template.ParseGlob("templates/*.html"))
)

// mormorMain represents the main program execution.
type mormorMain struct {
	metadata *metadataService
	enduser  *enduserService
}

func (m *mormorMain) Run() {

	interrupt := make(chan os.Signal, 1)
	errors := make(chan error)

	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	log.Println("Mormor is starting.")

	go func() {
		if err := m.metadata.Start(); err != nil {
			errors <- fmt.Errorf("error starting metadata service: %v", err)
		}
	}()

	go func() {
		if err := m.enduser.Start(); err != nil {
			errors <- fmt.Errorf("error starting enduser service: %v", err)
		}
	}()

	select {
	case <-interrupt:
		break
	case err := <-errors:
		log.Println(err)
		break
	}

	signal.Stop(interrupt)

	log.Println("Mormor is shutting down.")

	if err := m.enduser.Stop(); err != nil {
		log.Printf("error stopping enduser service: %v", err)
	}

	if err := m.metadata.Stop(); err != nil {
		log.Printf("error stopping metadata service: %v", err)
	}

}

func newMormorMain(ms *metadataService, es *enduserService) *mormorMain {
	return &mormorMain{
		metadata: ms,
		enduser:  es,
	}
}

func main() {

	var (
		enduserAddr  = flag.String("enduser-addr", ":7000", "end-user service listening address")
		metadataAddr = flag.String("metadata-addr", ":7001", "metadata service listening address")
		metadataDB   = flag.String("metadata-db", "metadata.db", "metadata database")
		metadataNS   = flag.String("metadata-ns", "", "metadata namespace (RDF resource base URI)")
		//adminAddr    = flag.String("admin-addr", ":7007", "admin interface listening address")
	)

	flag.Parse()

	metadata := newMetadataService(*metadataAddr, *metadataDB, *metadataNS)
	metadata.searchService = newSearchService()
	enduser := newEndUserService(*enduserAddr, metadata)

	m := newMormorMain(metadata, enduser)

	m.Run()
}

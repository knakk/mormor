package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

type service interface {
	String() string
	Start() error
	Stop() error
}

// mormorMain represents the main program execution.
type mormorMain struct {
	services []service
}

func (m *mormorMain) Run() {

	errors := make(chan error)
	interrupt := make(chan os.Signal, 1)

	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	log.Println("Mormor is starting.")

	// Start all services.
	for _, s := range m.services {
		go func(s service) {
			if err := s.Start(); err != nil {
				log.Printf("error starting %v service: %v", s, err)
				errors <- err
			}
		}(s)
	}

	select {
	case <-interrupt:
		break
	case <-errors:
		break
	}

	log.Println("Mormor is shutting down.")

	// Shutdown all services. Do it in reverse order, since there
	// are dependencies between them.
	for i := len(m.services) - 1; i >= 0; i-- {
		if err := m.services[i].Stop(); err != nil {
			log.Printf("error stopping %v service: %v", m.services[i], err)
		}
	}

}

func newMormorMain(services ...service) *mormorMain {
	m := mormorMain{
		services: services,
	}
	return &m
}

func main() {

	var (
		enduserAddr  = flag.String("enduser-addr", ":7000", "end-user service listening address")
		metadataAddr = flag.String("metadata-addr", ":7001", "metadata service listening address")
		metadataDB   = flag.String("metadata-db", "metadata.db", "metadata database")
		metadataNS   = flag.String("metadata-ns", "", "metadata namespace (RDF resource base URI)")
	)

	flag.Parse()

	metadata := newMetadataService(*metadataAddr, *metadataDB, *metadataNS)
	enduser := newEndUserService(*enduserAddr, metadata)

	m := newMormorMain(metadata, enduser)

	m.Run()
}

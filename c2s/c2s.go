/*
 * Copyright (c) 2018 Miguel Ángel Ortuño.
 * See the LICENSE file for more information.
 */

package c2s

import (
	"sync"

	"github.com/ortuman/jackal/component"
	"github.com/ortuman/jackal/log"
	"github.com/ortuman/jackal/module"
	"github.com/pkg/errors"
)

const (
	streamMailboxSize   = 64
	iqResultMailboxSize = 32
)

const (
	streamNamespace           = "http://etherx.jabber.org/streams"
	tlsNamespace              = "urn:ietf:params:xml:ns:xmpp-tls"
	compressProtocolNamespace = "http://jabber.org/protocol/compress"
	bindNamespace             = "urn:ietf:params:xml:ns:xmpp-bind"
	sessionNamespace          = "urn:ietf:params:xml:ns:xmpp-session"
	saslNamespace             = "urn:ietf:params:xml:ns:xmpp-sasl"
	blockedErrorNamespace     = "urn:xmpp:blocking:errors"
)

var (
	mu          sync.RWMutex
	servers     = make(map[string]*server)
	shutdownCh  = make(chan chan struct{})
	initialized bool
)

// Initialize initializes c2s sub system spawning a connection listener
// for every server configuration.
func Initialize(srvConfigurations []Config, mods *module.Modules, comps *component.Components) {
	mu.Lock()
	if initialized {
		mu.Unlock()
		return
	}
	if len(srvConfigurations) == 0 {
		log.Error(errors.New("at least one c2s configuration is required"))
		return
	}
	// initialize all servers
	for i := 0; i < len(srvConfigurations); i++ {
		if _, err := initializeServer(&srvConfigurations[i], mods, comps); err != nil {
			log.Fatalf("%v", err)
		}
	}
	initialized = true
	mu.Unlock()

	// wait until shutdown...
	doneCh := <-shutdownCh

	mu.Lock()
	// close all servers
	for k, srv := range servers {
		if err := srv.shutdown(); err != nil {
			log.Error(err)
		}
		delete(servers, k)
	}
	close(doneCh)
	initialized = false
	mu.Unlock()
}

// Shutdown closes every server listener.
// This method should be used only for testing purposes.
func Shutdown() {
	ch := make(chan struct{})
	shutdownCh <- ch
	<-ch
}

func initializeServer(cfg *Config, mods *module.Modules, comps *component.Components) (*server, error) {
	srv := &server{cfg: cfg, mods: mods, comps: comps}
	servers[cfg.ID] = srv
	go srv.start()
	return srv, nil
}

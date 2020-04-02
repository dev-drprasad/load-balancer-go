package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type key int

const retriesKey key = iota
const statusDown bool = false
const statusUp bool = true

type arrayFlags []string

func (i *arrayFlags) String() string {
	return strings.Join(*i, ",")
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

type Backend struct {
	URL          *url.URL
	Alive        bool
	mux          sync.RWMutex
	ReverseProxy *httputil.ReverseProxy
}

func (b *Backend) setIsAlive(isAlive bool) {
	b.mux.Lock()
	b.Alive = isAlive
	b.mux.Unlock()
}

func (b *Backend) isAlive() bool {
	var isAlive bool
	b.mux.RLock()
	isAlive = b.Alive
	b.mux.RUnlock()
	return isAlive
}

func (b *Backend) updateStatus() {
	timeout := 1 * time.Second
	conn, err := net.DialTimeout("tcp", b.URL.Host, timeout)
	if err != nil {
		log.Printf("[%s] status: down \n", b.URL)
		b.setIsAlive(statusDown)
		return
	}
	_ = conn.Close()

	log.Printf("[%s] status: up \n", b.URL)
	b.setIsAlive(statusUp)
}

type ServerPool struct {
	backends []*Backend
	current  uint64
}

func (sp *ServerPool) addBackend(b *Backend) {
	sp.backends = append(sp.backends, b)
}

func (sp *ServerPool) nextIndex() int {
	return int(atomic.AddUint64(&sp.current, 1)) % len(sp.backends)
}

func (sp *ServerPool) getNextPeer() *Backend {
	nextIndex := sp.nextIndex()

	temp := nextIndex + len(sp.backends)
	for i := nextIndex; i < temp; i++ {
		backendIndex := i % len(sp.backends)

		backend := sp.backends[backendIndex]
		if !backend.isAlive() {
			continue
		}

		if nextIndex != backendIndex {
			atomic.StoreUint64(&sp.current, uint64(backendIndex))
		}
		return backend
	}

	return nil
}

func (sp *ServerPool) markBackendStatus(serverURL *url.URL, status bool) {
	for _, backend := range sp.backends {
		if backend.URL.String() == serverURL.String() {
			backend.setIsAlive(status)
		}
	}
}

func (sp *ServerPool) runHealthCheck() {
	for _, backend := range sp.backends {
		backend.updateStatus()
	}
}

func getLoadbalancerHandler(sp *ServerPool) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		peer := sp.getNextPeer()
		if peer == nil {
			http.Error(w, "Service not available", http.StatusServiceUnavailable)
			return
		}

		peer.ReverseProxy.ServeHTTP(w, r)
		return
	}
}

func main() {
	var serverList arrayFlags
	var port int
	flag.Var(&serverList, "backend", "Load balanced backend")
	flag.IntVar(&port, "port", 4000, "Port to serve")
	flag.Parse()

	if len(serverList) == 0 {
		log.Fatal("Please provide one or more backends to load balance")
	}

	var sp ServerPool

	for _, serverURLStr := range serverList {
		serverURL, err := url.Parse(serverURLStr)

		if err != nil {
			log.Fatal(err)
		}

		proxy := httputil.NewSingleHostReverseProxy(serverURL)
		proxy.Transport = &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   1 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			TLSHandshakeTimeout: 1 * time.Second,
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
			log.Printf("[%s] %s\n", serverURL.Host, e.Error())

			var retries int
			retries, _ = r.Context().Value(retriesKey).(int)

			if retries < 3 {
				time.Sleep(time.Millisecond * time.Duration(10))
				ctx := context.WithValue(r.Context(), retriesKey, retries+1)
				proxy.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			sp.markBackendStatus(serverURL, statusDown)

			getLoadbalancerHandler(&sp)(w, r)

		}

		sp.addBackend(&Backend{
			URL:          serverURL,
			Alive:        true,
			ReverseProxy: proxy,
		})

		log.Printf("configured server: %s\n", serverURL)

	}

	var numOfRequests int
	server := http.Server{
		Addr: fmt.Sprintf(":%d", port),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			numOfRequests++
			getLoadbalancerHandler(&sp)(w, r)
		}),
	}

	go func() {
		for {
			log.Println("num of requests: ", numOfRequests)
			sp.runHealthCheck()
			time.Sleep(10 * time.Second)
		}
	}()

	server.ListenAndServe()

}

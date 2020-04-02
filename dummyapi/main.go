package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
)

func main() {
	var serviceNo int
	var port int
	var errorProbability int
	flag.IntVar(&serviceNo, "serviceNo", 0, "service no")
	flag.IntVar(&port, "port", 8000, "port")
	flag.IntVar(&errorProbability, "errorProbability", 0, "error probability")
	flag.Parse()

	log.Println(serviceNo, port)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if rand.Intn(100) >= errorProbability {
			w.Write([]byte(fmt.Sprintf("%d", serviceNo)))
		} else {
			w.WriteHeader(500)
		}
	})

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))

}

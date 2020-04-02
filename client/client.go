package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

func main() {
	var interval int
	var concurrency int
	flag.IntVar(&interval, "interval", 250, "interval in milli second")
	flag.IntVar(&concurrency, "concurrency", 1, "concurrency")
	flag.Parse()

	for {
		for i := 1; i <= concurrency; i++ {
			go func() {
				resp, err := http.Get("http://localhost:8000")
				if err != nil {
					log.Println(err)
					return
				}

				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Println(err)
					return
				}

				log.Printf("status: %d response: %s\n", resp.StatusCode, string(body))
			}()
		}
		time.Sleep(time.Millisecond * time.Duration(interval))
	}
}

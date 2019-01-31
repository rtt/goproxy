package main

import (
	"flag"
	"fmt"
	//"math/rand"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/valyala/fasthttp"
)

var (
	addr     = flag.String("addr", ":8080", "TCP address to listen to")
	compress = flag.Bool("compress", false, "Whether to enable transparent response compression")
	url_map  = make(map[string]map[string]string)
)

func requestHandler(ctx *fasthttp.RequestCtx) {
	fmt.Println("RequestURI:", string(ctx.RequestURI()))
	fmt.Println("URI:", ctx.URI())
	fmt.Println("Path:", string(ctx.Path()))
	fmt.Println("Host:", string(ctx.Host()))
	fmt.Println("Method:", string(ctx.Method()))

	// remove any port info
	host := string(ctx.Host())
	if i := strings.Index(host, ":"); i != -1 {
		host = host[:i]
	}

	fmt.Println("Canonical host is", host)
	fmt.Println("\n----\n\n")

	path := fmt.Sprintf("%s", ctx.Path())
	_, in_map := url_map[host][path]

	if string(ctx.Method()) == "GET" && in_map {
		// we only ever redirect GETs
		// TODO: we should really think hard about the above assumption
		fmt.Println("Redirect...")

		// TODO: how do we derive the protocol?
		protocol := "https"
		redirect_location := fmt.Sprintf("%s://%s%s", protocol, host, path)

		ctx.SetStatusCode(fasthttp.StatusMovedPermanently)
		ctx.Response.Header.Set("Location", redirect_location)
		ctx.Response.Header.Set("X-Redirector", "redirect")

	} else {

		fmt.Println("Proxy...")

		// response should indicate we proxied it
		ctx.Response.Header.Set("X-Redirector", "proxy")

		// start building a request
		client := &http.Client{
			// do not follow redirects.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
        		return http.ErrUseLastResponse
    		},
		}

		// rebuild URL to request
		fq_url := fmt.Sprintf("%s://%s%s", "http", host, ctx.Path())

		// TODO: chunked responses?
		req, _ := http.NewRequest("GET", fq_url, nil)

		// copy headers from ingress request into proxy request
		ctx.Request.Header.VisitAll(func (key, value []byte) {
			req.Header.Set(string(key), string(value))
		})

		resp, err := client.Do(req)
		if err != nil {
			// todo?
			panic(err)
		}
		defer resp.Body.Close()

		b, err := ioutil.ReadAll(resp.Body)

		if err != nil {
			log.Fatal(err)
			// todo?
			panic(err)
		}

		for k, v := range resp.Header {
              ctx.Response.Header.Set(string(k), string(v[0]))
      	}

      	ctx.SetStatusCode(resp.StatusCode)

		body := string(b)

		fmt.Fprintf(ctx, "body")
		fmt.Println(len(body))
	}
}

func main() {

	// load some stuff into url_map
	url_map["vogue.co.uk"] = make(map[string]string)
	url_map["localhost"] = make(map[string]string)
	url_map["vogue.co.uk"]["/foo"] = "/bar"
	url_map["localhost"]["/foo"] = "/bar"

	h := requestHandler
	if *compress {
		h = fasthttp.CompressHandler(h)
	}

	fmt.Println("Starting...")
	if err := fasthttp.ListenAndServe(*addr, h); err != nil {
		log.Fatalf("Error in ListenAndServe: %s", err)
	}

}

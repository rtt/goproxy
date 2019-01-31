package main

import (
	"bytes"
	"flag"
	"fmt"
	//"math/rand"
	"io/ioutil"
	//	"io"
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

	// TODO: dunno what fastly calls this header, so im just gonna make it up
	ingress_protocol := ctx.Request.Header.Peek("X-Forwarded-Proto")
	if ingress_protocol == nil {
		// assume http? TODO
		ingress_protocol = []byte("http")
	}

	if string(ctx.Method()) == "GET" && in_map {
		// we only ever redirect GETs
		// TODO: we should really think hard about the above assumption
		fmt.Println("Mode: Redirect")

		redirect_location := fmt.Sprintf("%s://%s%s", ingress_protocol, host, path)

		ctx.SetStatusCode(fasthttp.StatusMovedPermanently)
		ctx.Response.Header.Set("Location", redirect_location)
		ctx.Response.Header.Set("X-Redirector", "redirect")

	} else {

		fmt.Println("Mode: Proxy")

		// response should indicate we proxied it
		ctx.Response.Header.Set("X-Redirector", "proxy")

		// start building a request
		client := &http.Client{
			// do not follow redirects; if there's a redirect issued upstream
			// then we should spit that request out as-is. i think.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		// rebuild URL to request
		proxy_url := fmt.Sprintf("%s://%s%s", ingress_protocol, host, ctx.Path())

		fmt.Println("Protocol:", string(ingress_protocol))
		fmt.Println("URL:", proxy_url)
		fmt.Println("Method:", string(ctx.Method()))

		// copy any body data
		ingress_body := ctx.PostBody()
		if len(ingress_body) == 0 {
			ingress_body = nil
			fmt.Println("Body: nil")
		} else {
			fmt.Println("Body:", string(ingress_body))
		}

		// TODO: how do we handle chunked responses
		proxy_req, _ := http.NewRequest(string(ctx.Method()), proxy_url, bytes.NewBuffer(ingress_body))

		// copy headers from ingress request into proxy request
		ctx.Request.Header.VisitAll(func(key, value []byte) {
			proxy_req.Header.Set(string(key), string(value))
			fmt.Println("Header:", string(key), ":", string(value))
		})

		proxy_resp, err := client.Do(proxy_req)
		if err != nil {
			// todo?
			panic(err)
		}
		defer proxy_resp.Body.Close()

		b, err := ioutil.ReadAll(proxy_resp.Body)

		if err != nil {
			log.Fatal(err)
			// todo?
			panic(err)
		}

		body := string(b)

		// copy response headers back to egress response
		for k, v := range proxy_resp.Header {
			ctx.Response.Header.Set(string(k), string(v[0]))
		}

		// copy status code back to egress response
		ctx.SetStatusCode(proxy_resp.StatusCode)

		// copy proxy response back to egress response
		fmt.Fprintf(ctx, body)
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

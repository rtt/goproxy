package main

// TODO: Http2?
// TODO: reduce conversions of []byte to string, as that has a non-zero cost.
// TODO: gzip during proxy phase
// TODO: what about CONNECT requests?

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/valyala/fasthttp"
	"log"
	"net"
	"strings"
)

var (
	addr     = flag.String("addr", ":8080", "TCP address to listen to")
	compress = flag.Bool("compress", false, "Whether to enable transparent response compression")
	url_map  = make(map[string]map[string]string)
	skip_headers = []string{"Server",}
)

func in_array(needle string, haystack []string) bool {
	for _, v := range haystack {
		if needle == v {
			return true
		}
	}
	return false
}

func requestHandler(ctx *fasthttp.RequestCtx) {

	// remove any port info
	host := string(ctx.Host())
	if i := strings.Index(host, ":"); i != -1 {
		host = host[:i]
	}

	path := fmt.Sprintf("%s", ctx.Path())
	_, in_map := url_map[host][path]

	// TODO: dunno what fastly calls this header, so im just gonna make it up
	ingress_protocol := ctx.Request.Header.Peek("X-Forwarded-Proto")
	if ingress_protocol == nil {
		// assume http? TODO
		ingress_protocol = []byte("http")
	}

	// todo: do we want to check the method here?
	if in_map {
		// we only ever redirect GETs
		// TODO: we should really think hard about the above assumption

		redirect_location := fmt.Sprintf("%s://%s%s", ingress_protocol, host, path)

		ctx.SetStatusCode(fasthttp.StatusMovedPermanently)
		ctx.Response.Header.Set("Location", redirect_location)
		ctx.Response.Header.Set("X-Redirector", "redirect")

		fmt.Println("[redirect] 301", host, path, "->", redirect_location)

	} else {

		// response should indicate we proxied it
		ctx.Response.Header.Set("X-Redirector", "proxy")

		// rebuild URL to request
		proxy_url := fmt.Sprintf("%s://%s%s", ingress_protocol, host, ctx.Path())

		// start building a request
		client := &fasthttp.Client{}

		proxy_request := fasthttp.AcquireRequest()
		proxy_response := fasthttp.AcquireResponse()

		proxy_request.SetRequestURI(proxy_url)

		// copy any body data
		ingress_body := ctx.PostBody()
		if len(ingress_body) == 0 {
			ingress_body = nil
		}

		// set body
		proxy_request.SetBodyString(string(ingress_body))

		// set request method
		proxy_request.Header.SetMethod(string(ctx.Method()))

		// copy headers from ingress request into proxy request
		ctx.Request.Header.VisitAll(func(key, value []byte) {
			proxy_request.Header.Add(string(key), string(value))
		})

		client.Do(proxy_request, proxy_response)

		b := proxy_response.Body()

		body := string(b)

		// copy response headers back to egress response
		h_count := 0
		proxy_response.Header.VisitAll(func(key, value []byte) {
			//fmt.Println(string(key), string(value))
			k := string(key)
			if in_array(k, skip_headers) {
				ctx.Response.Header.Add(k, string(value))
				h_count++
				fmt.Println(k)
			}
		})

		// copy status code back to egress response
		ctx.SetStatusCode(proxy_response.StatusCode())

		// copy proxy response back to egress response
		fmt.Fprintf(ctx, body)

		// we're done, i think?

		fmt.Println(fmt.Sprintf("[proxy] %s, status=%d, %d bytes, %d headers", proxy_url, proxy_response.StatusCode(), len(body), h_count))
	}
}

func client_conns(listener net.Listener) chan net.Conn {
    ch := make(chan net.Conn)
    i := 0
    go func() {
        for {
            client, _ := listener.Accept()
            if client == nil {
                fmt.Printf("Couldn't accept connection")
                continue
            }
            i++
            fmt.Printf("%d: %v <-> %v\n", i, client.LocalAddr(), client.RemoteAddr())
            ch <- client
        }
    }()
    return ch
}

func handle_conn(client net.Conn) {
    b := bufio.NewReader(client)
    for {
        line, err := b.ReadBytes('\n')
        if err != nil {
            break
        }
        client.Write(line)
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

	// spin the server up in a routine
	go func () {
		if err := fasthttp.ListenAndServe(*addr, h); err != nil {
			log.Fatalf("Error in ListenAndServe: %s", err)
		}
	}()

	// make a socket server to listen for commands
	// this may well be redis-esque in the end?
	server, _ := net.Listen("tcp", ":3107")
	if server == nil {
        panic("couldn't start listening")
    }
	conns := client_conns(server)
    for {
        go handle_conn(<-conns)
    }
}

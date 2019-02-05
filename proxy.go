package main

// TODO: Http2?
// TODO: reduce conversions of []byte to string, as that has a non-zero cost.
// TODO: gzip during proxy phase
// TODO: what about CONNECT requests?

// what does 301 do?
// it does some reverse proxying
// it does some 301-ing
// it does some 404ing

// [default]
// 	hsts_redirect = true
// 	[paths]
// 		url = "/*"
// 		response = 404






[vogue.co.uk]
* https://vogue.co.uk



import (
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/valyala/fasthttp"
	"log"
	"net"
	"strings"
)

const (
	server_addr         = "224.0.0.1:9999"
	max_datagram_size = 8192
)

var (
	addr         = flag.String("addr", "0.0.0.0", "TCP address to listen to")
	port         = flag.String("port", "8192", "TCP port to listen on")
	compress     = flag.Bool("compress", false, "Whether to enable transparent response compression")
	url_map      = make(map[string]map[string]string)
	current_map  = make(map[string]map[string]string)
	skip_headers = []string{"Server"}
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

func reset_map(umap map[string]map[string]string) {
	log.Print("Resetting map. Current length ", len(umap))
	umap = make(map[string]map[string]string)
	log.Print("Map length now ", len(umap))
}

func populate_url_map() {
	new_url_map := make(map[string]map[string]string)

	log.Print("Populating map...")
	new_url_map["vogue.co.uk"] = make(map[string]string)
	new_url_map["localhost"] = make(map[string]string)
	new_url_map["vogue.co.uk"]["/foo"] = "/bar"
	new_url_map["localhost"]["/foo"] = "/bar"
	log.Print(len(new_url_map), " items in map")

	log.Print("Switching in new map...")
	url_map = new_url_map
}

func multi_handler(src *net.UDPAddr, num_bytes int, bytes []byte) {
	//log.Println(num_bytes, "bytes read from", src)
	log.Println(hex.Dump(bytes[:num_bytes]))
}

func serve_multicast_udp(listen_addr string, handler func(*net.UDPAddr, int, []byte)) {
	addr, err := net.ResolveUDPAddr("udp", listen_addr)

	if err != nil {
		log.Fatal(err)
	}

	listener, err := net.ListenMulticastUDP("udp", nil, addr)
	listener.SetReadBuffer(max_datagram_size)

	for {
		b := make([]byte, max_datagram_size)
		n, src, err := listener.ReadFromUDP(b)
		if err != nil {
			log.Fatal("ReadFromUDP failed:", err)
		}
		handler(src, n, b)
	}
}

func main() {

	// load some stuff into url_map
	populate_url_map()

	log.Print(len(url_map), " urls in map")

	handler := requestHandler
	if *compress {
		handler = fasthttp.CompressHandler(handler)
	}

	*addr = fmt.Sprintf("%s:%s", *addr, *port)

	// spin the server up in a routine
	go func() {
		log.Print("HTTP listening on ", *addr)
		if err := fasthttp.ListenAndServe(*addr, handler); err != nil {
			log.Fatalf("Error in ListenAndServe: %s", err)
		}
	}()

	log.Print("Multicast listening on ", server_addr)
	serve_multicast_udp(server_addr, multi_handler)
}

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	dtypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

const portLabel = "proxy.port"

// proxyTransport is shared across all routes so backend connections pool.
var proxyTransport http.RoundTripper = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   10,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   5 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
}

type route struct {
	target *url.URL
	proxy  *httputil.ReverseProxy
}

type router struct {
	mu     sync.RWMutex
	routes map[string]*route // key: container name (lowercased)
	domain string            // e.g. "test.com"
}

func newRouter(domain string) *router {
	return &router{
		routes: make(map[string]*route),
		domain: strings.ToLower(strings.TrimPrefix(domain, ".")),
	}
}

func (r *router) set(name string, target *url.URL) {
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()
		},
		Transport: proxyTransport,
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			log.Printf("proxy error for %s -> %s: %v", req.Host, target, err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}
	r.mu.Lock()
	r.routes[strings.ToLower(name)] = &route{target: target, proxy: rp}
	r.mu.Unlock()
	log.Printf("route set: %s.%s -> %s", name, r.domain, target)
}

func (r *router) delete(name string) {
	r.mu.Lock()
	if _, ok := r.routes[strings.ToLower(name)]; ok {
		delete(r.routes, strings.ToLower(name))
		log.Printf("route removed: %s.%s", name, r.domain)
	}
	r.mu.Unlock()
}

func (r *router) lookup(host string) *route {
	host = strings.ToLower(host)
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	suffix := "." + r.domain
	if !strings.HasSuffix(host, suffix) {
		return nil
	}
	sub := strings.TrimSuffix(host, suffix)
	if sub == "" || strings.Contains(sub, ".") {
		// only single-level subdomains route to a container
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.routes[sub]
}

func (r *router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	rt := r.lookup(req.Host)
	if rt == nil {
		http.Error(w, "no route for "+req.Host, http.StatusNotFound)
		return
	}
	rt.proxy.ServeHTTP(w, req)
}

// pickPort returns the chosen container-side port for routing.
// Honors the `proxy.port` label, otherwise picks the lowest exposed TCP port.
func pickPort(c *dtypes.ContainerJSON) (int, error) {
	if v, ok := c.Config.Labels[portLabel]; ok {
		p, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || p <= 0 || p > 65535 {
			return 0, fmt.Errorf("invalid %s label %q on %s", portLabel, v, c.Name)
		}
		return p, nil
	}
	var best int
	for p := range c.Config.ExposedPorts {
		if p.Proto() != "tcp" {
			continue
		}
		n := p.Int()
		if best == 0 || n < best {
			best = n
		}
	}
	if best == 0 {
		return 0, fmt.Errorf("no exposed TCP port and no %s label on %s", portLabel, c.Name)
	}
	return best, nil
}

// pickIP returns a reachable container IP from its NetworkSettings.
func pickIP(c *dtypes.ContainerJSON) (string, error) {
	if c.NetworkSettings == nil {
		return "", errors.New("no network settings")
	}
	for _, n := range c.NetworkSettings.Networks {
		if n.IPAddress != "" {
			return n.IPAddress, nil
		}
	}
	if c.NetworkSettings.IPAddress != "" {
		return c.NetworkSettings.IPAddress, nil
	}
	return "", errors.New("no IP address on any network")
}

// containerName strips the leading slash Docker returns on names.
func containerName(raw string) string {
	return strings.TrimPrefix(raw, "/")
}

func register(ctx context.Context, cli *client.Client, r *router, id string) {
	insp, err := cli.ContainerInspect(ctx, id)
	if err != nil {
		log.Printf("inspect %s: %v", id, err)
		return
	}
	if insp.State == nil || !insp.State.Running {
		return
	}
	name := containerName(insp.Name)
	ip, err := pickIP(&insp)
	if err != nil {
		log.Printf("skip %s: %v", name, err)
		return
	}
	port, err := pickPort(&insp)
	if err != nil {
		log.Printf("skip %s: %v", name, err)
		return
	}
	u := &url.URL{Scheme: "http", Host: net.JoinHostPort(ip, strconv.Itoa(port))}
	r.set(name, u)
}

func syncAll(ctx context.Context, cli *client.Client, r *router) error {
	list, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return err
	}
	for _, c := range list {
		register(ctx, cli, r, c.ID)
	}
	return nil
}

func watch(ctx context.Context, cli *client.Client, r *router) {
	for {
		if err := streamEvents(ctx, cli, r); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("event stream error: %v (retrying in 2s)", err)
			time.Sleep(2 * time.Second)
		}
	}
}

func streamEvents(ctx context.Context, cli *client.Client, r *router) error {
	f := filters.NewArgs()
	f.Add("type", "container")
	msgs, errs := cli.Events(ctx, dtypes.EventsOptions{Filters: f})
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errs:
			return err
		case ev := <-msgs:
			name := containerName(ev.Actor.Attributes["name"])
			switch ev.Action {
			case events.ActionStart:
				register(ctx, cli, r, ev.Actor.ID)
			case events.ActionDie, events.ActionStop, events.ActionKill, events.ActionDestroy, events.ActionPause:
				if name != "" {
					r.delete(name)
				}
			case events.ActionUnPause:
				register(ctx, cli, r, ev.Actor.ID)
			}
		}
	}
}

func main() {
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		log.Fatal("DOMAIN env var is required (e.g. DOMAIN=test.com)")
	}
	addr := os.Getenv("LISTEN")
	if addr == "" {
		addr = ":80"
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("docker client: %v", err)
	}
	defer cli.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r := newRouter(domain)

	if err := syncAll(ctx, cli, r); err != nil {
		log.Fatalf("initial sync: %v", err)
	}
	go watch(ctx, cli, r)

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("doxy listening on %s, domain=*.%s", addr, r.domain)
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	case <-ctx.Done():
		log.Printf("shutdown signal received, draining...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}
}

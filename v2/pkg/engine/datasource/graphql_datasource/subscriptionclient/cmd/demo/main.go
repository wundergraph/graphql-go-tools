package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/xid"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource/subscriptionclient/client"
)

var colors = []string{
	"\033[31m", // red
	"\033[32m", // green
	"\033[33m", // yellow
	"\033[34m", // blue
	"\033[35m", // magenta
	"\033[36m", // cyan
}

const reset = "\033[0m"

type listenerStats struct {
	counter *atomic.Uint64
	color   string
	key     string
}

type stats struct {
	mu        sync.RWMutex
	listeners []*listenerStats
}

func newStats() *stats {
	return &stats{}
}

func (s *stats) register(key, color string) *atomic.Uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	ls := &listenerStats{
		counter: &atomic.Uint64{},
		color:   color,
		key:     key,
	}
	s.listeners = append(s.listeners, ls)
	return ls.counter
}

type deviation struct {
	key   string
	color string
	rate  float64
	diff  float64
}

func (s *stats) report(interval time.Duration) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const expected = 1.0
	const tolerance = 0.1

	var total float64
	var devs []deviation
	for _, ls := range s.listeners {
		count := ls.counter.Swap(0)
		rate := float64(count) / interval.Seconds()
		total += rate
		diff := rate - expected
		if diff < -tolerance || diff > tolerance {
			devs = append(devs, deviation{ls.key, ls.color, rate, diff})
		}
	}

	expectedTotal := float64(len(s.listeners)) * expected
	if len(devs) == 0 {
		fmt.Printf("  all %d listeners at expected rate\n", len(s.listeners))
	} else {
		sort.Slice(devs, func(i, j int) bool {
			return abs(devs[i].diff) > abs(devs[j].diff)
		})
		shown := min(len(devs), 10)
		for _, d := range devs[:shown] {
			fmt.Printf("  %s[%s]%s %.1f msg/s (expected %.1f)\n", d.color, d.key, reset, d.rate, expected)
		}
		if len(devs) > 10 {
			fmt.Printf("  ... and %d more deviations\n", len(devs)-10)
		}
	}
	fmt.Printf("  total: %.1f msg/s (expected %.1f)\n", total, expectedTotal)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

type vu struct {
	id    string
	color string
	opts  client.Options
}

func (v vu) format(format string, args ...any) string {
	msg := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s[%s]%s %s", v.color, v.id, reset, msg)
}

func main() {
	endpoint := flag.String("endpoint", "http://localhost:6001/query", "GraphQL endpoint")
	numVUs := flag.Int("vu", 2, "Virtual users (unique connections via InitPayload)")
	numSubs := flag.Int("subs", 3, "Unique subscriptions per VU (different extensions.subId)")
	numListeners := flag.Int("listeners", 2, "Listeners per subscription (fan-out)")
	transport := flag.String("transport", "ws", "Transport: ws, sse, or mixed")
	pprofAddr := flag.String("pprof", "localhost:6060", "pprof server address (empty to disable)")
	flag.Parse()

	if *pprofAddr != "" {
		go func() {
			log.Printf("pprof server: http://%s/debug/pprof/\n", *pprofAddr)
			log.Println(http.ListenAndServe(*pprofAddr, nil))
		}()
	}

	fmt.Printf("Config: vu=%d subs=%d listeners=%d transport=%s\n", *numVUs, *numSubs, *numListeners, *transport)
	fmt.Printf("Expected: subscriptions=%d total_listeners=%d\n",
		*numVUs**numSubs,
		*numVUs**numSubs**numListeners,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	run(ctx, *endpoint, *numVUs, *numSubs, *numListeners, *transport)
}

func run(ctx context.Context, endpoint string, numVUs, numSubs, numListeners int, transport string) {
	c := client.New(http.DefaultClient, http.DefaultClient)
	defer c.Close()

	st := newStats()
	var wg sync.WaitGroup

	for i := range numVUs {
		x := xid.New()
		id := fmt.Sprintf("%06x", x.Counter())

		// Determine transport for this VU
		var vuTransport client.TransportType
		switch transport {
		case "sse":
			vuTransport = client.TransportSSE
		case "mixed":
			if i%2 == 0 {
				vuTransport = client.TransportWS
			} else {
				vuTransport = client.TransportSSE
			}
		default: // "ws"
			vuTransport = client.TransportWS
		}

		opts := client.Options{
			Endpoint:    endpoint,
			Transport:   vuTransport,
			InitPayload: map[string]any{"user": id},
		}
		if vuTransport == client.TransportWS {
			opts.WSSubprotocol = client.SubprotocolGraphQLTransportWS
		}

		v := vu{
			id:    id,
			color: colors[i%len(colors)],
			opts:  opts,
		}

		for sub := range numSubs {
			for listener := range numListeners {
				wg.Add(1)
				key := fmt.Sprintf("%s/s%d/l%d", v.id, sub, listener)
				counter := st.register(key, v.color)
				go func(v vu, subId, listenerId int, counter *atomic.Uint64) {
					defer wg.Done()
					subscribe(ctx, c, v, subId, listenerId, counter)
				}(v, sub, listener, counter)
			}
		}
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.Tick(5 * time.Second):
				logStats(c, st, 5*time.Second)
			}
		}
	}()

	wg.Wait()
}

func logStats(c *client.Client, st *stats, interval time.Duration) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	cs := c.Stats()
	fmt.Printf("\n--- ws=%d sse=%d subs=%d listeners=%d | goroutines=%d alloc=%dMB ---\n",
		cs.WSConns, cs.SSEConns, cs.Subscriptions, cs.Listeners,
		runtime.NumGoroutine(),
		m.Alloc/1024/1024,
	)
	st.report(interval)
}

func subscribe(ctx context.Context, c *client.Client, v vu, subId, listenerId int, counter *atomic.Uint64) {
	req := &client.Request{
		Query: `subscription { time(timezone: "UTC") }`,
		Extensions: map[string]any{
			"subId": subId, // varies per subscription to prevent dedup across subs
		},
	}

	ch, cancel, err := c.Subscribe(ctx, req, v.opts)
	if err != nil {
		log.Print(v.format("s%d/l%d error: %v", subId, listenerId, err))
		return
	}
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				log.Print(v.format("s%d/l%d closed", subId, listenerId))
				return
			}
			if msg.Err != nil {
				log.Print(v.format("s%d/l%d error: %v", subId, listenerId, msg.Err))
				if msg.Done {
					return
				}
				continue
			}
			if msg.Done {
				log.Print(v.format("s%d/l%d complete", subId, listenerId))
				return
			}
			counter.Add(1)
		}
	}
}

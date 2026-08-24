// server-sim is a minimal fake "Internal Server" used only for local
// development and testing of the Store & Forward engine (internal/forwarder).
// It is not part of the gateway binary and is never deployed to production.
//
// It demonstrates the server-side half of Rule 7/10 (at-least-once
// delivery + idempotent processing): it deduplicates on gateway_id +
// sequence_id, so a batch retried after a lost ACK does not double-count.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"sync"
)

type entry struct {
	GatewayID      string   `json:"gateway_id"`
	SequenceID     int64    `json:"sequence_id"`
	DeviceID       int64    `json:"device_id"`
	DatapointID    int64    `json:"datapoint_id"`
	Value          *float64 `json:"value"`
	Quality        string   `json:"quality"`
	EventTimestamp string   `json:"event_timestamp"`
	Priority       string   `json:"priority"`
}

type key struct {
	gatewayID  string
	sequenceID int64
}

type store struct {
	mu       sync.Mutex
	seen     map[key]entry
	received []entry
}

func newStore() *store {
	return &store{seen: make(map[key]entry)}
}

func (s *store) ingest(batch []entry) (accepted, duplicates int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range batch {
		k := key{e.GatewayID, e.SequenceID}
		if _, dup := s.seen[k]; dup {
			duplicates++
			continue
		}
		s.seen[k] = e
		s.received = append(s.received, e)
		accepted++
	}
	return accepted, duplicates
}

func (s *store) all() []entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]entry, len(s.received))
	copy(out, s.received)
	return out
}

func main() {
	addr := flag.String("addr", ":9000", "listen address")
	flag.Parse()

	st := newStore()

	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var batch []entry
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		accepted, duplicates := st.ingest(batch)
		log.Printf("ingest: received=%d accepted=%d duplicates=%d", len(batch), accepted, duplicates)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"accepted": accepted, "duplicates": duplicates})
	})
	mux.HandleFunc("/received", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(st.all())
	})

	log.Printf("server-sim listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

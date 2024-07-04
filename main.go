package loadbalancer

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/pauljubcse/algorithm"
)

type Config struct {
	Addresses              []string `json:"addresses"`
	Backends               []string `json:"backends"`
	Weights                []int    `json:"weights"` // For weighted round robin
	BackendTimeout         int      `json:"backend_timeout"`
	ReadTimeout            int      `json:"read_timeout"`
	WriteTimeout           int      `json:"write_timeout"`
	UseServiceRegistry     bool     `json:"use_service_registry"`
	LoadBalancingAlgorithm string   `json:"load_balancing_algorithm"`
}

var (
	config Config
	// current uint32
	lb algorithm.LoadBalancer
)

// Load configuration from a file
func loadConfig(file string) error {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &config)
}

// Handlers
func root(w http.ResponseWriter, req *http.Request) {
	relay := &http.Client{
		Timeout: time.Duration(config.BackendTimeout) * time.Second,
	}

	var backend string = lb.NextBackend(req)
	backendReq, err := http.NewRequest(req.Method, backend, req.Body)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	for header, values := range req.Header {
		for _, value := range values {
			backendReq.Header.Add(header, value)
		}
	}

	resp, err := relay.Do(backendReq)
	if err != nil {
		fmt.Fprintf(w, "Backend %s Didn't Respond in Time", backend)
		//http.Error(w, "Backend Didn't Respond in Time", http.StatusGatewayTimeout)
		return
	}
	defer resp.Body.Close()

	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		http.Error(w, "Failed to get Response", http.StatusInternalServerError)
	}
	fmt.Println(backend)
	//Dummy
	// w.WriteHeader(http.StatusAccepted)
	// fmt.Fprint(w, backend)
}
func main() {
	configFile := flag.String("config", "config.json", "Path to configuration file")
	flag.Parse()

	err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	fmt.Print(config.Backends)
	switch config.LoadBalancingAlgorithm {
	case "round_robin":
		lb = algorithm.NewRoundRobin(config.Backends)
	case "weighted_round_robin":
		lb = algorithm.NewWeightedRoundRobin(config.Backends, config.Weights)
	// case "least_connection":
	// 	lb = algorithm.NewLeastConnection(config.Backends)
	case "ip_hash":
		lb = algorithm.NewIPHash(config.Backends)
	default:
		log.Fatalf("Unknown load balancing algorithm: %v", config.LoadBalancingAlgorithm)
	}

	for _, address := range config.Addresses {
		srv := &http.Server{
			Addr:         address,
			Handler:      http.HandlerFunc(root),
			ReadTimeout:  time.Duration(config.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(config.WriteTimeout) * time.Second,
		}

		go func(s *http.Server) {
			log.Printf("Starting server on %s", s.Addr)
			if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Could not listen on %s: %v", s.Addr, err)
			}
		}(srv)
	}

	select {}
}

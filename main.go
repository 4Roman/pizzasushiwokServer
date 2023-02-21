package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"pizzasushiwokServer/config"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type NeoCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type NeoCountRequest struct {
	NeoCount []NeoCount `json:"neo_count"`
}

type NeoCountResponse struct {
	ElementCount int `json:"element_count"`
}

func main() {
	gracefulShutDown := make(chan os.Signal, 1)
	signal.Notify(gracefulShutDown, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.GetInstance()
	conn, err := pgx.Connect(pgx.ConnConfig{
		Host:     cfg.DbHost,
		Port:     stringToUint16(cfg.DbPort),
		Database: cfg.DbName,
		User:     cfg.DbUser,
		Password: cfg.DbPassword,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	http.HandleFunc("/neo/count", neoCountHandler(ctx, conn))
	go http.ListenAndServe(":8080", nil)
	<-gracefulShutDown
}

func neoCountHandler(ctx context.Context, conn *pgx.Conn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			neoCountGETHandler(ctx, w, r)
		} else if r.Method == http.MethodPost {
			neoCountPOSTHandler(ctx, conn, w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func neoCountPOSTHandler(ctx context.Context, conn *pgx.Conn, w http.ResponseWriter, r *http.Request) {
	var neoCounts NeoCountRequest
	err := json.NewDecoder(r.Body).Decode(&neoCounts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, nc := range neoCounts.NeoCount {
		date, err := time.Parse("2006-01-02", nc.Date)
		if err != nil {
			log.Fatal(err)
		}
		_, err = conn.Exec("INSERT INTO neo_counts (date, neo) VALUES ($1, $2) ON CONFLICT (date) DO UPDATE SET neo = $2", date, nc.Count)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusCreated)
}

func neoCountGETHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	dates, ok := r.URL.Query()["dates"]
	if !ok || len(dates) < 1 {
		http.Error(w, "Missing dates parameter", http.StatusBadRequest)
		return
	}

	var totalCount int
	var URL = "https://api.nasa.gov/neo/rest/v1/feed"
	var apiKey = config.GetApiKey()

	var wg sync.WaitGroup // используем WaitGroup для ожидания завершения всех горутин
	wg.Add(len(dates))

	// используем каналы для передачи результата из горутин в основную функцию
	results := make(chan NeoCountResponse, len(dates))

	for _, date := range dates {
		go func(date string) {
			defer wg.Done()

			u, err := url.Parse(URL)
			if err != nil {
				log.Printf("Error parsing URL: %v", err)
				return
			}
			q := u.Query()
			q.Set("start_date", date)
			q.Set("end_date", date)
			q.Set("detailed", "true")
			q.Set("api_key", apiKey)
			u.RawQuery = q.Encode()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if err != nil {
				log.Printf("Error making request: %v", err)
				return
			}
			cl := http.Client{}
			resp, err := cl.Do(req)
			if err != nil {
				log.Printf("Error sending request: %v", err)
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("Error reading response body: %v", err)
				return
			}

			var n NeoCountResponse
			if err := json.Unmarshal(body, &n); err != nil {
				log.Printf("Error unmarshalling response: %v", err)
				return
			}
			results <- n
		}(date)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for n := range results {
		totalCount += n.ElementCount
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"total_count": %d}`, totalCount)
}

func stringToUint16(s string) uint16 {
	i, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		log.Print("error: stringToUint16")
		return 0
	}
	return uint16(i)
}

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/bmizerany/pq"
	"io"
	"log"
	"net/http"
	"net/url"
	"pizzasushiwokServer/config"
	"sync"
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
	ctx := context.Background()
	cfg := config.GetInstance()
	print(fmt.Sprintf("postgres://%s:%s@%s/neo_count?sslmode=disable", cfg.DbUser, cfg.DbPassword, cfg.DbHost))
	db, err := sql.Open("postgres", fmt.Sprintf("postgres://%s:%s@%s/neo_count?sslmode=disable", cfg.DbUser, cfg.DbPassword, cfg.DbHost))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	http.HandleFunc("/neo/count", neoCountHandler(ctx, db))
	http.ListenAndServe(":8080", nil)
}

func neoCountHandler(ctx context.Context, db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			neoCountGETHandler(ctx, w, r)
		} else if r.Method == "POST" {
			neoCountPOSTHandler(ctx, db, w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func neoCountPOSTHandler(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var neoCounts NeoCountRequest
	err := json.NewDecoder(r.Body).Decode(&neoCounts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, nc := range neoCounts.NeoCount {
		_, err = db.Exec(`INSERT INTO neo_count (date, count) VALUES ($1, $2) ON CONFLICT (date) DO UPDATE SET count = $2`, nc.Date, nc.Count)
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

			resp, err := http.Get(u.String())
			if err != nil {
				log.Printf("Error making request: %v", err)
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

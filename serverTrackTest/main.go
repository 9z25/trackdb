package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	_ "github.com/lib/pq"
	"github.com/pirsch-analytics/pirsch"
)

const (
	DB_USER     = "testuser"
	DB_PASSWORD = "secret123"
	DB_NAME     = "sammy"
)

// DB set up
func setupDB() *sql.DB {
	dbinfo := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", DB_USER, DB_PASSWORD, DB_NAME)
	db, err := sql.Open("postgres", dbinfo)
	if err != nil {
		panic(err)
	}

	return db
}

func main() {

	db := setupDB()

	// Create a new Postgres store to save statistics and hits.
	store := pirsch.NewPostgresStore(db, nil)

	// Set up a default tracker with a salt.
	// This will buffer and store hits and generate sessions by default.
	tracker := pirsch.NewTracker(store, "salt", nil)

	// Create a new process and run it each day on midnight (UTC) to process the stored hits.
	// The processor also cleans up the hits.
	processor := pirsch.NewProcessor(store)
	pirsch.RunAtMidnight(func() {
		if err := processor.Process(); err != nil {
			panic(err)
		}
	})

	// Create a handler to serve traffic.
	// We prevent tracking resources by checking the path. So a file on /my-file.txt won't create a new hit
	// but all page calls will be tracked.
	http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hit" {
			go tracker.Hit(r, nil)
		}

		r.RequestURI = ""
		r.Header.Set("X-Forwarded-For", r.RemoteAddr)
		r.Header.Set("Referer", r.URL.String())
		URL, _ := url.Parse("http://localhost:8000/shopping")
		r.URL.Scheme = URL.Scheme
		r.URL.Host = URL.Host
		r.URL.Path = URL.Path

		client := &http.Client{}
		// Step 3: execute request
		log.Println(r.URL.String())
		resp, err := client.Do(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Step 4: copy payload to response writer
		copyHeader(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		resp.Body.Close()

	}))

	// And finally, start the server.
	// We don't flush hits on shutdown but you should add that in a real application by calling Tracker.Flush().
	log.Println("Starting server on port 8080...")
	http.ListenAndServe(":8080", nil)

}

// copyHeader and singleJoiningSlash are copy from "/net/http/httputil/reverseproxy.go"
func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

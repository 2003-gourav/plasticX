package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"

    "github.com/2003-gourav/plasticX/backend/db" // adjust import path
)

func main() {
    // Initialize database
    if err := db.Init(); err != nil {
        log.Fatal("Database init failed:", err)
    }
    defer db.DB.Close()

    http.HandleFunc("/", home)
    http.HandleFunc("/health", health)
    http.HandleFunc("/markets", getMarkets)

    log.Println("Server starting on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}

func home(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "PLASTIC backend running")
}

func health(w http.ResponseWriter, r *http.Request) {
    // Check database connection
    if err := db.DB.Ping(); err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "error": err.Error()})
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func getMarkets(w http.ResponseWriter, r *http.Request) {
    rows, err := db.DB.Query("SELECT id, name, x_reserve, y_reserve FROM markets")
    if err != nil {
        log.Printf("Query error: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    type Market struct {
        ID       int     `json:"id"`
        Name     string  `json:"name"`
        XReserve float64 `json:"x_reserve"`
        YReserve float64 `json:"y_reserve"`
    }

    var markets []Market
    for rows.Next() {
        var m Market
        if err := rows.Scan(&m.ID, &m.Name, &m.XReserve, &m.YReserve); err != nil {
            log.Printf("Scan error: %v", err)
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }
        markets = append(markets, m)
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(markets)
}

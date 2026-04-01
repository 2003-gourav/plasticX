package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"

    "github.com/2003-gourav/plasticX/backend/db" // import path
)
type Market struct {
        ID       int     `json:"id"`
        Name     string  `json:"name"`
        XReserve float64 `json:"x_reserve"`
        YReserve float64 `json:"y_reserve"`
        Fee      float64 `json:"fee"`
    }
type CreateMarketRequest struct {
    Name     string  `json:"name"`
    XReserve float64 `json:"x_reserve"`
    YReserve float64 `json:"y_reserve"`
    Fee      float64 `json:"fee"`
}
func main() {
    // Initialize database
    if err := db.Init(); err != nil {
        log.Fatal("Database init failed:", err)
    }
    defer db.DB.Close()

    http.HandleFunc("/", home)
    http.HandleFunc("/health", health)
    http.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        getMarkets(w, r)
    case http.MethodPost:
        createMarket(w, r)
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
})

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
    rows, err := db.DB.Query("SELECT id, name, x_reserve, y_reserve, fee FROM markets")
    if err != nil {
        log.Printf("Query error: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    

    var markets []Market

    for rows.Next() {
        var m Market
        if err := rows.Scan(&m.ID, &m.Name, &m.XReserve, &m.YReserve, &m.Fee); err != nil {
            log.Printf("Scan error: %v", err)
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }
        markets = append(markets, m)
    }

    // THIS 
    if err := rows.Err(); err != nil {
        log.Printf("Rows iteration error: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")

    
    if markets == nil {
        markets = []Market{}
    }

    json.NewEncoder(w).Encode(markets)
}
func createMarket(w http.ResponseWriter, r *http.Request) {
    var req CreateMarketRequest

    // Decode JSON
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    // Validation
    if req.Name == "" {
        http.Error(w, "name is required", http.StatusBadRequest)
        return
    }

    if req.XReserve <= 0 || req.YReserve <= 0 {
        http.Error(w, "reserves must be positive", http.StatusBadRequest)
        return
    }

    if req.Fee <= 0 || req.Fee >= 0.1 {
        http.Error(w, "fee must be between 0 and 0.1", http.StatusBadRequest)
        return
    }

    // Insert into DB
    var id int
    err := db.DB.QueryRow(
        "INSERT INTO markets(name, x_reserve, y_reserve, fee) VALUES($1, $2, $3, $4) RETURNING id",
        req.Name, req.XReserve, req.YReserve, req.Fee,
    ).Scan(&id)

    if err != nil {
        log.Printf("Insert error: %v", err)
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }

    // Response
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)

    json.NewEncoder(w).Encode(map[string]interface{}{
        "id":        id,
        "name":      req.Name,
        "x_reserve": req.XReserve,
        "y_reserve": req.YReserve,
        "fee":       req.Fee,
    })
}

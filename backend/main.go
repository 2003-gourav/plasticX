package main

import (
    "encoding/json"
    "fmt"
    "net/http"
)

func home(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "PLASTIC backend running")
}

func health(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "status": "ok",
    })
}

func getMarkets(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")

    markets := []map[string]interface{}{
        {
            "id":        1,
            "name":      "ETH/USDC",
            "x_reserve": 1000,
            "y_reserve": 2000,
        },
    }

    json.NewEncoder(w).Encode(markets)
}

func main() {
    http.HandleFunc("/", home)
    http.HandleFunc("/health", health)
    http.HandleFunc("/markets", getMarkets)

    fmt.Println("Server starting on :8080")
    http.ListenAndServe(":8080", nil)
}

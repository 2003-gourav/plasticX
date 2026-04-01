package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "database/sql"

    "github.com/2003-gourav/plasticX/backend/db" // import path
)
type Market struct {
        ID       int     `json:"id"`
        Name     string  `json:"name"`
        XReserve float64 `json:"x_reserve"`
        YReserve float64 `json:"y_reserve"`
        Fee      float64 `json:"fee"`
    }
    
type TradeRequest struct {
    MarketID  int     `json:"market_id"`
    Direction string  `json:"direction"` // "buy" or "sell"
    Amount    float64 `json:"amount"`
}

type TradeResponse struct {
    MarketID   int     `json:"market_id"`
    Direction  string  `json:"direction"`
    AmountIn   float64 `json:"amount_in"`
    AmountOut  float64 `json:"amount_out"`
    FeePaid    float64 `json:"fee_paid"`
    Price      float64 `json:"price"`
    NewPrice   float64 `json:"new_price"`
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
	http.HandleFunc("/trade", trade)

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
func trade(w http.ResponseWriter, r *http.Request) {
    var req TradeRequest

    // Decode JSON
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    // Validation
    if req.MarketID <= 0 {
        http.Error(w, "market_id is required", http.StatusBadRequest)
        return
    }
    if req.Direction != "buy" && req.Direction != "sell" {
        http.Error(w, "direction must be 'buy' or 'sell'", http.StatusBadRequest)
        return
    }
    if req.Amount <= 0 {
        http.Error(w, "amount must be positive", http.StatusBadRequest)
        return
    }

    // Start transaction
    tx, err := db.DB.Begin()
    if err != nil {
        log.Printf("Begin transaction error: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }
    defer tx.Rollback() // safety net

    //Lock market row
    var x, y, fee, treasury float64
    err = tx.QueryRow(
        "SELECT x_reserve, y_reserve, fee, treasury FROM markets WHERE id = $1 FOR UPDATE",
        req.MarketID,
    ).Scan(&x, &y, &fee, &treasury)
    if err == sql.ErrNoRows {
        http.Error(w, "Market not found", http.StatusNotFound)
        return
    }
    if err != nil {
        log.Printf("Query error: %v", err)
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }

    //Compute AMM swap
    var amountIn, amountOut, feePaid, newX, newY float64
    var direction string

    if req.Direction == "buy" {
        direction = "buy"
        amountIn = req.Amount
        feePaid = amountIn * fee
        netAmount := amountIn - feePaid
        newY = y + netAmount
        newX = (x * y) / newY
        amountOut = x - newX
    } else {
        direction = "sell"
        amountIn = req.Amount
        newX = x + amountIn
        newY = (x * y) / newX
        grossOut := y - newY
        feePaid = grossOut * fee
        amountOut = grossOut - feePaid
    }

    // Update treasury
    treasury += feePaid

    // Automatic buyback if treasury exceeds 10% of y_reserve
    buybackThreshold := 0.1 * y
    if treasury > buybackThreshold {
        buybackAmount := treasury
        // Buy back base token from pool using constant product formula
        // Treat treasury as y_amount to buy x
        newX_bb := (newX * newY) / (newY + buybackAmount) // new X after buyback
        xBought := newX - newX_bb
        newX = newX_bb
        treasury = 0 // treasury used entirely for buyback
        log.Printf("Buyback triggered: used %.4f treasury to buy %.4f base tokens", buybackAmount, xBought)
    }

    // Update market reserves and treasury
    _, err = tx.Exec(
        "UPDATE markets SET x_reserve = $1, y_reserve = $2, treasury = $3 WHERE id = $4",
        newX, newY, treasury, req.MarketID,
    )
    if err != nil {
        log.Printf("Update market error: %v", err)
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }

    // Record trade
    avgPrice := amountOut / amountIn
    _, err = tx.Exec(
        `INSERT INTO trades (market_id, direction, amount_in, amount_out, fee_paid, price)
         VALUES ($1, $2, $3, $4, $5, $6)`,
        req.MarketID, direction, amountIn, amountOut, feePaid, avgPrice,
    )
    if err != nil {
        log.Printf("Insert trade error: %v", err)
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }

    //Commit transaction
    if err := tx.Commit(); err != nil {
        log.Printf("Commit error: %v", err)
        http.Error(w, "Transaction failed", http.StatusInternalServerError)
        return
    }

    // Respond to client
    newPrice := newY / newX
    resp := TradeResponse{
        MarketID:  req.MarketID,
        Direction: direction,
        AmountIn:  amountIn,
        AmountOut: amountOut,
        FeePaid:   feePaid,
        Price:     avgPrice,
        NewPrice:  newPrice,
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(resp)
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

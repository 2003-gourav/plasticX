package main

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "strconv"
    "strings"

    "github.com/2003-gourav/plasticX/backend/amm"
    "github.com/2003-gourav/plasticX/backend/db"
    "github.com/2003-gourav/plasticX/backend/models"
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
const MinInitialLiquidity = 1000.0 // in base asset units
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
    http.HandleFunc("/markets/", getPrice) // will handle /markets/{id}/price
    http.HandleFunc("/memes", createMeme)
    http.HandleFunc("/markets/", func(w http.ResponseWriter, r *http.Request) {
        if strings.HasSuffix(r.URL.Path, "/memes") {
            getMemesByMarket(w, r)
        } else if strings.HasSuffix(r.URL.Path, "/price") {
            getPrice(w, r)
        } else {
            http.NotFound(w, r)
        }
    })
    http.HandleFunc("/attention", addAttention)

    log.Println("Server starting on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}

func home(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "PLASTIC backend running")
}

func health(w http.ResponseWriter, r *http.Request) {
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
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

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
    if req.XReserve < MinInitialLiquidity {
        http.Error(w, fmt.Sprintf("initial x_reserve must be at least %.2f", MinInitialLiquidity), http.StatusBadRequest)
        return
    }

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

func trade(w http.ResponseWriter, r *http.Request) {
    var req TradeRequest
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
    defer tx.Rollback()

    // Lock market row
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

    // Create a pool with current reserves and fee
    pool := amm.NewPool(x, y, fee)

    var amountIn, amountOut, feePaid float64
    var direction string

    if req.Direction == "buy" {
        direction = "buy"
        amountIn = req.Amount
        // Declare dx, then assign from the swap
        var dx float64
        dx, feePaid = pool.SwapYToX(amountIn)
        amountOut = dx
    } else {
        direction = "sell"
        amountIn = req.Amount
        // Declare dy, then assign from the swap
        var dy float64
        dy, feePaid = pool.SwapXToY(amountIn)
        amountOut = dy
    }

    // Update new reserves from the pool
    newX := pool.X
    newY := pool.Y

    // Add fee to treasury
    treasury += feePaid

    // Automatic buyback if treasury exceeds 10% of y_reserve
    buybackThreshold := 0.1 * y
    if treasury > buybackThreshold {
        buybackAmount := treasury
        // Use a fee‑less swap: treat treasury as y to buy x
        newX_bb := (newX * newY) / (newY + buybackAmount)
        xBought := newX - newX_bb
        newX = newX_bb
        treasury = treasury - buybackAmount
        log.Printf("Buyback triggered: used %.4f treasury to buy %.4f base tokens", buybackAmount, xBought)
    }

    // Update market
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

    // Commit transaction
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
// getPrice handles GET /markets/{id}/price
func getPrice(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    // Extract ID from path, e.g., /markets/1/price
    path := strings.TrimPrefix(r.URL.Path, "/markets/")
    idStr := strings.TrimSuffix(path, "/price")
    id, err := strconv.Atoi(idStr)
    if err != nil {
        http.Error(w, "Invalid market ID", http.StatusBadRequest)
        return
    }

    var x, y float64
    err = db.DB.QueryRow("SELECT x_reserve, y_reserve FROM markets WHERE id = $1", id).Scan(&x, &y)
    if err == sql.ErrNoRows {
        http.Error(w, "Market not found", http.StatusNotFound)
        return
    }
    if err != nil {
        log.Printf("Price query error: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    pool := amm.NewPool(x, y, 0) // fee not needed for price
    price := pool.Price()

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]float64{"price": price})
}
// POST /memes
func createMeme(w http.ResponseWriter, r *http.Request) {
    var req struct {
        CreatorID string `json:"creator_id"`
        MarketID  int    `json:"market_id"`
        ImageURL  string `json:"image_url"`
        Caption   string `json:"caption"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    // Validate market exists
    var exists bool
    err := db.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM markets WHERE id = $1)", req.MarketID).Scan(&exists)
    if err != nil || !exists {
        http.Error(w, "Market not found", http.StatusBadRequest)
        return
    }

    hash := models.ComputeContentHash(req.ImageURL, req.Caption)

    var memeID int
    err = db.DB.QueryRow(`
        INSERT INTO memes(creator_id, market_id, image_url, caption, content_hash)
        VALUES($1, $2, $3, $4, $5) RETURNING id`,
        req.CreatorID, req.MarketID, req.ImageURL, req.Caption, hash,
    ).Scan(&memeID)
    if err != nil {
        log.Printf("Insert meme error: %v", err)
        http.Error(w, "Database error (maybe duplicate hash)", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(map[string]int{"id": memeID})
}

// GET /markets/{id}/memes
func getMemesByMarket(w http.ResponseWriter, r *http.Request) {
    path := strings.TrimPrefix(r.URL.Path, "/markets/")
    idStr := strings.TrimSuffix(path, "/memes")
    marketID, err := strconv.Atoi(idStr)
    if err != nil {
        http.Error(w, "Invalid market ID", http.StatusBadRequest)
        return
    }

    rows, err := db.DB.Query(`
        SELECT id, creator_id, image_url, caption, created_at
        FROM memes WHERE market_id = $1 ORDER BY created_at DESC`, marketID)
    if err != nil {
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    var memes []models.Meme
    for rows.Next() {
        var m models.Meme
        if err := rows.Scan(&m.ID, &m.CreatorID, &m.ImageURL, &m.Caption, &m.CreatedAt); err != nil {
            http.Error(w, "Scan error", http.StatusInternalServerError)
            return
        }
        memes = append(memes, m)
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(memes)
}

// POST /attention (increment raw metrics for a meme)
func addAttention(w http.ResponseWriter, r *http.Request) {
    var req struct {
        MemeID     int `json:"meme_id"`
        Views      int `json:"views"`
        UniqueViews int `json:"unique_views"`
        Reposts    int `json:"reposts"`
        Derivatives int `json:"derivatives"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    // Upsert: increment counts for current timestamp (simplified – you may want to aggregate by hour)
    _, err := db.DB.Exec(`
        INSERT INTO meme_attention (meme_id, views, unique_views, reposts, derivatives, timestamp)
        VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
        ON CONFLICT (meme_id, timestamp) DO UPDATE SET
            views = meme_attention.views + EXCLUDED.views,
            unique_views = meme_attention.unique_views + EXCLUDED.unique_views,
            reposts = meme_attention.reposts + EXCLUDED.reposts,
            derivatives = meme_attention.derivatives + EXCLUDED.derivatives`,
        req.MemeID, req.Views, req.UniqueViews, req.Reposts, req.Derivatives,
    )
    if err != nil {
        log.Printf("Attention update error: %v", err)
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
}

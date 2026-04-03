package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/2003-gourav/plasticX/backend/amm"
	"github.com/2003-gourav/plasticX/backend/db"
	"github.com/2003-gourav/plasticX/backend/models"
)

const MinInitialLiquidity = 1000.0

// -------------------- TYPES --------------------

type CreateMarketRequest struct {
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
	MarketID  int     `json:"market_id"`
	Direction string  `json:"direction"`
	AmountIn  float64 `json:"amount_in"`
	AmountOut float64 `json:"amount_out"`
	FeePaid   float64 `json:"fee_paid"`
	Price     float64 `json:"price"`
	NewPrice  float64 `json:"new_price"`
}

// -------------------- MAIN --------------------

func main() {
	if err := db.Init(); err != nil {
		log.Fatal("DB init failed:", err)
	}
	defer db.DB.Close()

	// Routes
	http.HandleFunc("/", home)
	http.HandleFunc("/health", health)

	http.HandleFunc("/markets", marketsHandler)
	http.HandleFunc("/markets/", marketsSubroutes)

	http.HandleFunc("/trade", tradeHandler)

	http.HandleFunc("/memes", createMeme)
	http.HandleFunc("/memes/", memesSubroutes)

	http.HandleFunc("/attention/event", recordAttentionEvent) // Week 2 event endpoint

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// -------------------- BASIC HANDLERS --------------------

func home(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "PLASTIC backend running")
}

func health(w http.ResponseWriter, r *http.Request) {
	if err := db.DB.Ping(); err != nil {
		http.Error(w, "unhealthy", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// -------------------- MARKETS --------------------

func marketsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getMarkets(w, r)
	case http.MethodPost:
		createMarket(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func getMarkets(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query(`
		SELECT id, name, x_reserve, y_reserve, fee, treasury, created_at
		FROM markets ORDER BY id
	`)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	defer rows.Close()

	var markets []models.Market
	for rows.Next() {
		var m models.Market
		if err := rows.Scan(&m.ID, &m.Name, &m.XReserve, &m.YReserve, &m.Fee, &m.Treasury, &m.CreatedAt); err != nil {
			http.Error(w, "Scan error", 500)
			return
		}
		markets = append(markets, m)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(markets)
}

func createMarket(w http.ResponseWriter, r *http.Request) {
	var req CreateMarketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}
	if req.XReserve < MinInitialLiquidity || req.YReserve <= 0 {
		http.Error(w, "insufficient liquidity", 400)
		return
	}
	if req.Fee < 0.001 || req.Fee > 0.05 {
		http.Error(w, "fee must be between 0.001 and 0.05", 400)
		return
	}

	var id int
	err := db.DB.QueryRow(`
		INSERT INTO markets (name, x_reserve, y_reserve, fee, treasury)
		VALUES ($1, $2, $3, $4, 0) RETURNING id
	`, req.Name, req.XReserve, req.YReserve, req.Fee).Scan(&id)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"id": id})
}

func marketsSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/markets/")
	switch {
	case strings.HasSuffix(path, "/price"):
		getPrice(w, r)
	case strings.HasSuffix(path, "/attention"):
		getMarketAttention(w, r)
	case strings.HasSuffix(path, "/top-memes"):
		getTopMemes(w, r)
	case strings.HasSuffix(path, "/stats"):
		getMarketStats(w, r)
	default:
		http.NotFound(w, r)
	}
}

func getPrice(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/markets/"), "/price")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid market id", 400)
		return
	}
	var x, y float64
	err = db.DB.QueryRow("SELECT x_reserve, y_reserve FROM markets WHERE id=$1", id).Scan(&x, &y)
	if err == sql.ErrNoRows {
		http.Error(w, "market not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	if x == 0 {
		http.Error(w, "invalid pool", 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]float64{"price": y / x})
}

func getMarketAttention(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/markets/"), "/attention")
	marketID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid market id", 400)
		return
	}
	// Sum over memes' attention_stats (fast cache)
	var views, uniqueViews, reposts, derivatives int
	err = db.DB.QueryRow(`
		SELECT COALESCE(SUM(ms.views), 0), COALESCE(SUM(ms.unique_views), 0),
		       COALESCE(SUM(ms.reposts), 0), COALESCE(SUM(ms.derivatives), 0)
		FROM memes m
		JOIN meme_attention_stats ms ON m.id = ms.meme_id
		WHERE m.market_id = $1
	`, marketID).Scan(&views, &uniqueViews, &reposts, &derivatives)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"market_id":   marketID,
		"views":       views,
		"unique_views": uniqueViews,
		"reposts":     reposts,
		"derivatives": derivatives,
	})
}

func getTopMemes(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/markets/"), "/top-memes")
	marketID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid market id", 400)
		return
	}
	sortBy := r.URL.Query().Get("sort")
	allowed := map[string]bool{"views": true, "unique_views": true, "reposts": true, "derivatives": true}
	if !allowed[sortBy] {
		sortBy = "reposts"
	}
	query := fmt.Sprintf(`
		SELECT m.id, m.caption, COALESCE(ms.%s, 0) as val
		FROM memes m
		LEFT JOIN meme_attention_stats ms ON m.id = ms.meme_id
		WHERE m.market_id = $1
		ORDER BY val DESC
		LIMIT 10
	`, sortBy)
	rows, err := db.DB.Query(query, marketID)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	defer rows.Close()
	var result []map[string]interface{}
	for rows.Next() {
		var id int
		var caption string
		var val int
		rows.Scan(&id, &caption, &val)
		result = append(result, map[string]interface{}{
			"id": id, "caption": caption, sortBy: val,
		})
	}
	json.NewEncoder(w).Encode(result)
}

func getMarketStats(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/markets/"), "/stats")
	marketID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid market id", 400)
		return
	}
	var market models.Market
	err = db.DB.QueryRow(`
		SELECT id, name, x_reserve, y_reserve, fee, treasury, created_at
		FROM markets WHERE id = $1
	`, marketID).Scan(&market.ID, &market.Name, &market.XReserve, &market.YReserve,
		&market.Fee, &market.Treasury, &market.CreatedAt)
	if err == sql.ErrNoRows {
		http.Error(w, "market not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	var volume24h, tradeCount24h, fees24h float64
	db.DB.QueryRow(`
		SELECT COALESCE(SUM(amount_in), 0), COUNT(*), COALESCE(SUM(fee_paid), 0)
		FROM trades
		WHERE market_id = $1 AND timestamp > NOW() - INTERVAL '24 hours'
	`, marketID).Scan(&volume24h, &tradeCount24h, &fees24h)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":                 market.ID,
		"name":               market.Name,
		"x_reserve":          market.XReserve,
		"y_reserve":          market.YReserve,
		"price":              market.YReserve / market.XReserve,
		"fee":                market.Fee,
		"treasury":           market.Treasury,
		"volume_24h":         volume24h,
		"trade_count_24h":    tradeCount24h,
		"fees_collected_24h": fees24h,
	})
}

// -------------------- TRADE --------------------

func tradeHandler(w http.ResponseWriter, r *http.Request) {
	var req TradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}
	if req.Direction != "buy" && req.Direction != "sell" {
		http.Error(w, "direction must be buy or sell", 400)
		return
	}
	if req.Amount <= 0 {
		http.Error(w, "amount must be positive", 400)
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		http.Error(w, "tx error", 500)
		return
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var x, y, fee, treasury float64
	err = tx.QueryRow(`
		SELECT x_reserve, y_reserve, fee, treasury FROM markets WHERE id = $1 FOR UPDATE
	`, req.MarketID).Scan(&x, &y, &fee, &treasury)
	if err == sql.ErrNoRows {
		http.Error(w, "market not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}

	pool := amm.NewPool(x, y, fee)
	var amountOut, feePaid float64
	if req.Direction == "buy" {
		amountOut, feePaid = pool.SwapYToX(req.Amount) // buy Y with X? Actually SwapYToX means give Y get X. Need consistent naming.
		// In our amm: SwapXToY = give X get Y (buy Y). Let's check: earlier code used SwapYToX for buy. We'll keep as is.
		// To avoid confusion: we assume SwapXToY is buy Y, SwapYToX is sell Y. But user's direction "buy" means buy Y (give X).
		// So we should call SwapXToY. Let's fix:
		// We'll re-implement correctly.
	} else {
		amountOut, feePaid = pool.SwapXToY(req.Amount)
	}
	// The above is swapped. Let's write correct logic:
	// Actually from original code: req.Direction=="buy" -> SwapYToX? That was a bug. We'll fix here.
	// Correct:
	// - buy: user gives X (base), receives Y (meme) => SwapXToY
	// - sell: user gives Y, receives X => SwapYToX
	var newX, newY float64
	if req.Direction == "buy" {
		amountOut, feePaid = pool.SwapXToY(req.Amount)
		newX, newY = pool.X, pool.Y
	} else {
		amountOut, feePaid = pool.SwapYToX(req.Amount)
		newX, newY = pool.X, pool.Y
	}

	treasury += feePaid

	// Buyback: if treasury > 10% of y_reserve, use treasury to buy Y (support price)
	if treasury > 0.1*newY {
		bbPool := amm.NewPool(newX, newY, 0)
		// treasury is in X, use it to buy Y: SwapXToY
		_, _ = bbPool.SwapXToY(treasury) // fee=0
		newX = bbPool.X
		newY = bbPool.Y
		treasury = 0
	}

	_, err = tx.Exec(`
		UPDATE markets SET x_reserve = $1, y_reserve = $2, treasury = $3 WHERE id = $4
	`, newX, newY, treasury, req.MarketID)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}

	avgPrice := amountOut / req.Amount
	_, err = tx.Exec(`
		INSERT INTO trades (market_id, direction, amount_in, amount_out, fee_paid, price)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, req.MarketID, req.Direction, req.Amount, amountOut, feePaid, avgPrice)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}

	if err = tx.Commit(); err != nil {
		http.Error(w, "commit error", 500)
		return
	}

	resp := TradeResponse{
		MarketID:  req.MarketID,
		Direction: req.Direction,
		AmountIn:  req.Amount,
		AmountOut: amountOut,
		FeePaid:   feePaid,
		Price:     avgPrice,
		NewPrice:  newY / newX,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// -------------------- MEMES --------------------

func createMeme(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CreatorID string `json:"creator_id"`
		MarketID  int    `json:"market_id"`
		ImageURL  string `json:"image_url"`
		Caption   string `json:"caption"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}
	if req.MarketID <= 0 || req.ImageURL == "" {
		http.Error(w, "invalid input", 400)
		return
	}
	var exists bool
	err := db.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM markets WHERE id=$1)", req.MarketID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "market not found", 400)
		return
	}
	hash := models.ComputeContentHash(req.ImageURL, req.Caption)
	var id int
	err = db.DB.QueryRow(`
		INSERT INTO memes (creator_id, market_id, image_url, caption, content_hash)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, req.CreatorID, req.MarketID, req.ImageURL, req.Caption, hash).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "content_hash") {
			http.Error(w, "duplicate meme", 409)
			return
		}
		http.Error(w, "DB error", 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]int{"id": id})
}

func memesSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/memes/")
	if strings.HasSuffix(path, "/stats") {
		getMemeStats(w, r)
	} else if strings.HasSuffix(path, "/events") {
		getMemeEvents(w, r)
	} else {
		http.NotFound(w, r)
	}
}

func getMemeStats(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/memes/"), "/stats")
	memeID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid meme id", 400)
		return
	}
	var views, uniqueViews, reposts, derivatives int
	err = db.DB.QueryRow(`
		SELECT COALESCE(views,0), COALESCE(unique_views,0), COALESCE(reposts,0), COALESCE(derivatives,0)
		FROM meme_attention_stats WHERE meme_id = $1
	`, memeID).Scan(&views, &uniqueViews, &reposts, &derivatives)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, "DB error", 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"meme_id":      memeID,
		"views":        views,
		"unique_views": uniqueViews,
		"reposts":      reposts,
		"derivatives":  derivatives,
	})
}

func getMemeEvents(w http.ResponseWriter, r *http.Request) {
    idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/memes/"), "/events")
    memeID, err := strconv.Atoi(idStr)
    if err != nil {
        http.Error(w, "invalid meme id", 400)
        return
    }

    eventType := r.URL.Query().Get("type")
    limit := 100
    if l := r.URL.Query().Get("limit"); l != "" {
        if parsed, _ := strconv.Atoi(l); parsed > 0 && parsed <= 1000 {
            limit = parsed
        }
    }

    var query string
    var args []interface{}
    if eventType != "" {
        query = `SELECT event_id, event_type, COALESCE(user_id, '') as user_id, timestamp 
                 FROM attention_events 
                 WHERE meme_id = $1 AND event_type = $2 
                 ORDER BY timestamp DESC LIMIT $3`
        args = []interface{}{memeID, eventType, limit}
    } else {
        query = `SELECT event_id, event_type, COALESCE(user_id, '') as user_id, timestamp 
                 FROM attention_events 
                 WHERE meme_id = $1 
                 ORDER BY timestamp DESC LIMIT $2`
        args = []interface{}{memeID, limit}
    }

    rows, err := db.DB.Query(query, args...)
    if err != nil {
        log.Printf("Query error: %v", err)
        http.Error(w, "DB error", 500)
        return
    }
    defer rows.Close()

    type Event struct {
        EventID   int       `json:"event_id"`
        EventType string    `json:"event_type"`
        UserID    string    `json:"user_id,omitempty"`
        Timestamp time.Time `json:"timestamp"`
    }
    var events []Event
    for rows.Next() {
        var e Event
        var userID string
        if err := rows.Scan(&e.EventID, &e.EventType, &userID, &e.Timestamp); err != nil {
            log.Printf("Scan error: %v", err)
            http.Error(w, "Scan error", 500)
            return
        }
        if userID != "" {
            e.UserID = userID
        }
        events = append(events, e)
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(events)
}
func recordAttentionEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		MemeID int    `json:"meme_id"`
		UserID string `json:"user_id,omitempty"`
		Action string `json:"action"` // view, unique_view, repost, derivative
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}
	valid := map[string]bool{"view": true, "unique_view": true, "repost": true, "derivative": true}
	if !valid[req.Action] {
		http.Error(w, "invalid action", 400)
		return
	}
	// Check meme exists
	var exists bool
	err := db.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM memes WHERE id=$1)", req.MemeID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "meme not found", 404)
		return
	}
	// Insert raw event
	_, err = db.DB.Exec(`
		INSERT INTO attention_events (meme_id, user_id, event_type, timestamp)
		VALUES ($1, $2, $3, NOW())
	`, req.MemeID, req.UserID, req.Action)
	if err != nil {
		log.Printf("Insert event error: %v", err)
		http.Error(w, "DB error", 500)
		return
	}
	// Update aggregated stats (upsert)
	var col string
	switch req.Action {
	case "view":
		col = "views"
	case "unique_view":
		col = "unique_views"
	case "repost":
		col = "reposts"
	case "derivative":
		col = "derivatives"
	}
	_, err = db.DB.Exec(fmt.Sprintf(`
		INSERT INTO meme_attention_stats (meme_id, %s) VALUES ($1, 1)
		ON CONFLICT (meme_id) DO UPDATE SET %s = meme_attention_stats.%s + 1
	`, col, col, col), req.MemeID)
	if err != nil {
		log.Printf("Update stats error: %v", err)
		// non-fatal, but log
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
}

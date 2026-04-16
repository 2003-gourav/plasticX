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

// parseWindow maps query ?window= to a PostgreSQL INTERVAL literal (whitelist only; safe for SQL embedding).
func parseWindow(window string) (string, error) {
	switch window {
	case "5m":
		return "5 minutes", nil
	case "1h":
		return "1 hour", nil
	case "6h":
		return "6 hours", nil
	case "24h":
		return "24 hours", nil
	case "":
		return "1 hour", nil
	default:
		return "1 hour", nil
	}
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

	http.HandleFunc("/top-memes", getTopMemesByVelocity)
	http.HandleFunc("/trending-markets", getTrendingMarkets)

	go func() {
		for {
			refreshMemeSignals()
			refreshMarketSignals()
			time.Sleep(60 * time.Second)
		}
	}()

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func refreshMemeSignals() {
	query := `
		WITH last_hour AS (
			SELECT 
				meme_id,
				COUNT(*) FILTER (WHERE event_type = 'view') AS views_1h,
				COUNT(*) FILTER (WHERE event_type = 'repost') AS reposts_1h,
				COUNT(*) FILTER (WHERE event_type = 'derivative') AS derivatives_1h
			FROM attention_events
			WHERE timestamp > NOW() - INTERVAL '1 hour'
			GROUP BY meme_id
		),
		last_24h AS (
			SELECT 
				meme_id,
				COUNT(*) FILTER (WHERE event_type = 'view') AS views_24h
			FROM attention_events
			WHERE timestamp > NOW() - INTERVAL '24 hours'
			GROUP BY meme_id
		),
		combined AS (
			SELECT 
				COALESCE(lh.meme_id, l24.meme_id) AS meme_id,
				COALESCE(lh.views_1h, 0) AS views_1h,
				COALESCE(lh.reposts_1h, 0) AS reposts_1h,
				COALESCE(lh.derivatives_1h, 0) AS derivatives_1h,
				COALESCE(l24.views_24h, 0) AS views_24h,
				COALESCE(lh.views_1h, 0)::double precision AS velocity_1h
			FROM last_hour lh
			FULL OUTER JOIN last_24h l24 ON lh.meme_id = l24.meme_id
		)
		INSERT INTO meme_signals (meme_id, views_1h, views_24h, reposts_1h, derivatives_1h, velocity_1h, momentum, attention_score, updated_at)
		SELECT 
			c.meme_id,
			c.views_1h,
			c.views_24h,
			c.reposts_1h,
			c.derivatives_1h,
			c.velocity_1h,
			COALESCE(0.3 * c.velocity_1h + 0.7 * ms.momentum, c.velocity_1h) AS momentum,
			COALESCE(
				(c.views_1h::double precision * 1.0 + c.reposts_1h::double precision * 3.0 + c.derivatives_1h::double precision * 5.0) *
				(0.3 * c.velocity_1h + 0.7 * COALESCE(ms.momentum, c.velocity_1h)),
				(c.views_1h::double precision * 1.0 + c.reposts_1h::double precision * 3.0 + c.derivatives_1h::double precision * 5.0) * c.velocity_1h
			) AS attention_score,
			NOW()
		FROM combined c
		LEFT JOIN meme_signals ms ON c.meme_id = ms.meme_id
		ON CONFLICT (meme_id) DO UPDATE SET
			views_1h = EXCLUDED.views_1h,
			views_24h = EXCLUDED.views_24h,
			reposts_1h = EXCLUDED.reposts_1h,
			derivatives_1h = EXCLUDED.derivatives_1h,
			velocity_1h = EXCLUDED.velocity_1h,
			momentum = EXCLUDED.momentum,
			attention_score = EXCLUDED.attention_score,
			updated_at = EXCLUDED.updated_at
	`
	_, err := db.DB.Exec(query)
	if err != nil {
		log.Printf("Refresh meme signals error: %v", err)
		return
	}

	// Zero rows for memes with no attention_events in the last 24h (avoids stale views_24h / scores).
	zeroQuery := `
		UPDATE meme_signals ms
		SET 
			views_1h = 0,
			views_24h = 0,
			reposts_1h = 0,
			derivatives_1h = 0,
			velocity_1h = 0,
			momentum = 0,
			attention_score = 0,
			updated_at = NOW()
		WHERE NOT EXISTS (
			SELECT 1 FROM attention_events ae
			WHERE ae.meme_id = ms.meme_id
			AND ae.timestamp > NOW() - INTERVAL '24 hours'
		)
	`
	if _, err := db.DB.Exec(zeroQuery); err != nil {
		log.Printf("Zero stale meme_signals error: %v", err)
	}

	log.Println("Meme signals refreshed (views_1h, views_24h, velocity, momentum, attention_score)")
}

func refreshMarketSignals() {
	query := `
		INSERT INTO market_signals (market_id, total_attention_score, total_views_1h, market_velocity, market_momentum, updated_at)
		SELECT 
			m.market_id,
			COALESCE(SUM(ms.attention_score), 0) AS total_attention_score,
			COALESCE(SUM(ms.views_1h), 0) AS total_views_1h,
			COALESCE(SUM(ms.velocity_1h), 0) AS market_velocity,
			COALESCE(AVG(ms.momentum), 0) AS market_momentum,
			NOW()
		FROM memes m
		LEFT JOIN meme_signals ms ON m.id = ms.meme_id
		GROUP BY m.market_id
		ON CONFLICT (market_id) DO UPDATE SET
			total_attention_score = EXCLUDED.total_attention_score,
			total_views_1h = EXCLUDED.total_views_1h,
			market_velocity = EXCLUDED.market_velocity,
			market_momentum = EXCLUDED.market_momentum,
			updated_at = EXCLUDED.updated_at
	`
	_, err := db.DB.Exec(query)
	if err != nil {
		log.Printf("Refresh market signals error: %v", err)
	} else {
		log.Println("Market signals refreshed")
	}
}

func getTopMemesByVelocity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	rows, err := db.DB.Query(`
		SELECT m.id, m.caption, m.image_url, ms.views_1h, ms.velocity_1h, ms.reposts_1h, ms.derivatives_1h,
		       ms.momentum, ms.attention_score
		FROM meme_signals ms
		JOIN memes m ON ms.meme_id = m.id
		ORDER BY ms.attention_score DESC NULLS LAST
		LIMIT $1
	`, limit)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id int
		var caption, imageURL string
		var views, reposts, derivatives int
		var velocity, momentum, attention float64
		if err := rows.Scan(&id, &caption, &imageURL, &views, &velocity, &reposts, &derivatives, &momentum, &attention); err != nil {
			http.Error(w, "Scan error", 500)
			return
		}
		result = append(result, map[string]interface{}{
			"meme_id":          id,
			"caption":          caption,
			"image_url":        imageURL,
			"views_1h":         views,
			"velocity_1h":      velocity,
			"reposts_1h":       reposts,
			"derivatives_1h":   derivatives,
			"momentum":         momentum,
			"attention_score":  attention,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
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

	name := strings.TrimSpace(req.Name)
	if name == "" {
		http.Error(w, "name is required", 400)
		return
	}

	var exists bool
	err := db.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM markets WHERE name = $1)`, name).Scan(&exists)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	if exists {
		http.Error(w, "market with this name already exist", http.StatusConflict)
		return
	}

	var id int
	err = db.DB.QueryRow(`
		INSERT INTO markets (name, x_reserve, y_reserve, fee, treasury)
		VALUES ($1, $2, $3, $4, 0) RETURNING id
	`, name, req.XReserve, req.YReserve, req.Fee).Scan(&id)
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
	case strings.HasSuffix(path, "/attention-detail"):
    	getMarketAttentionDetail(w, r)
	case strings.HasSuffix(path, "/attention"):
		getMarketAttention(w, r)
	case strings.HasSuffix(path, "/top-memes"):
		getTopMemes(w, r)
	case strings.HasSuffix(path, "/stats"):
		getMarketStats(w, r)
	case strings.HasSuffix(path, "/signals"):
		getMarketSignals(w, r)
	case strings.HasSuffix(path, "/insights"):
		getMarketInsights(w, r)
	case strings.HasSuffix(path, "/trending"):
		getMarketTrending(w, r)
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

func getMarketSignals(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/markets/"), "/signals")
	marketID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid market id", 400)
		return
	}

	var exists bool
	if err := db.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM markets WHERE id = $1)`, marketID).Scan(&exists); err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	if !exists {
		http.Error(w, "market not found", 404)
		return
	}

	var signal models.MarketSignal
	err = db.DB.QueryRow(`
		SELECT 
			COALESCE(total_attention_score, 0),
			COALESCE(total_views_1h, 0),
			COALESCE(market_velocity, 0),
			COALESCE(market_momentum, 0),
			updated_at
		FROM market_signals
		WHERE market_id = $1
	`, marketID).Scan(
		&signal.TotalAttentionScore, &signal.TotalViews1h,
		&signal.MarketVelocity, &signal.MarketMomentum, &signal.UpdatedAt,
	)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, "DB error", 500)
		return
	}
	if err == sql.ErrNoRows {
		signal = models.MarketSignal{MarketID: marketID}
	}
	signal.MarketID = marketID

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(signal)
}

// getMarketInsights returns lifetime aggregates from meme_attention_stats plus ratio-style scores (not materialized signals).
func getMarketInsights(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/markets/"), "/insights")
	marketID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid market id", 400)
		return
	}

	var totalViews, totalUnique, totalReposts, totalDerivatives int
	err = db.DB.QueryRow(`
		SELECT COALESCE(SUM(ms.views),0), COALESCE(SUM(ms.unique_views),0),
		       COALESCE(SUM(ms.reposts),0), COALESCE(SUM(ms.derivatives),0)
		FROM memes m
		JOIN meme_attention_stats ms ON m.id = ms.meme_id
		WHERE m.market_id = $1
	`, marketID).Scan(&totalViews, &totalUnique, &totalReposts, &totalDerivatives)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}

	avgRepostRatio := 0.0
	derivativeRatio := 0.0
	viralityScore := 0.0
	if totalViews > 0 {
		avgRepostRatio = float64(totalReposts) / float64(totalViews)
		derivativeRatio = float64(totalDerivatives) / float64(totalViews)
		viralityScore = (float64(totalReposts)*models.AttentionWeightRepost + float64(totalDerivatives)*models.AttentionWeightDerivative) / (float64(totalViews) + 1)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"market_id":          marketID,
		"total_views":        totalViews,
		"total_unique_views": totalUnique,
		"total_reposts":      totalReposts,
		"total_derivatives":  totalDerivatives,
		"avg_repost_ratio":   avgRepostRatio,
		"derivative_ratio":   derivativeRatio,
		"virality_score":     viralityScore,
	})
}

func getTopMemes(w http.ResponseWriter, r *http.Request) {
    // Parse parameters
    sortBy := r.URL.Query().Get("sort")
    validSorts := map[string]string{
        "momentum":        "ms.momentum",
        "attention_score": "ms.attention_score",
        "views":           "ms.views_1h",
        "reposts":         "ms.reposts_1h",
        "derivatives":     "ms.derivatives_1h",
        "newest":          "m.created_at",
    }
    if _, ok := validSorts[sortBy]; !ok {
        sortBy = "momentum"
    }
    sortColumn := validSorts[sortBy]

    limit := 20
    if l := r.URL.Query().Get("limit"); l != "" {
        if parsed, _ := strconv.Atoi(l); parsed > 0 && parsed <= 100 {
            limit = parsed
        }
    }

    offset := 0
    if o := r.URL.Query().Get("offset"); o != "" {
        if parsed, _ := strconv.Atoi(o); parsed >= 0 {
            offset = parsed
        }
    }

    maxPerMarket := 3
    if m := r.URL.Query().Get("max_per_market"); m != "" {
        if parsed, _ := strconv.Atoi(m); parsed >= 1 && parsed <= 10 {
            maxPerMarket = parsed
        }
    }

    minMomentum := 0.0
    if mm := r.URL.Query().Get("min_momentum"); mm != "" {
        if parsed, _ := strconv.ParseFloat(mm, 64); parsed != 0 {
            minMomentum = parsed
        }
    }

    // Build query with market deduplication using row_number()
    query := fmt.Sprintf(`
        WITH ranked_memes AS (
            SELECT 
                m.id,
                m.caption,
                m.image_url,
                m.market_id,
                mk.name as market_name,
                ms.views_1h,
                ms.views_24h,
                ms.reposts_1h,
                ms.derivatives_1h,
                ms.velocity_1h,
                ms.momentum,
                ms.attention_score,
                m.created_at,
                ROW_NUMBER() OVER (PARTITION BY m.market_id ORDER BY %s DESC) as market_rank
            FROM memes m
            JOIN meme_signals ms ON m.id = ms.meme_id
            JOIN markets mk ON m.market_id = mk.id
            WHERE ms.momentum >= $1
        )
        SELECT 
            id, caption, image_url, market_id, market_name,
            views_1h, views_24h, reposts_1h, derivatives_1h,
            velocity_1h, momentum, attention_score, created_at
        FROM ranked_memes
        WHERE market_rank <= $2
        ORDER BY %s DESC
        LIMIT $3 OFFSET $4
    `, sortColumn, sortColumn)

    rows, err := db.DB.Query(query, minMomentum, maxPerMarket, limit, offset)
    if err != nil {
        log.Printf("Top memes query error: %v", err)
        http.Error(w, "DB error", 500)
        return
    }
    defer rows.Close()

    // Get total count for pagination
    var total int
    countQuery := `
        SELECT COUNT(DISTINCT m.id)
        FROM memes m
        JOIN meme_signals ms ON m.id = ms.meme_id
        WHERE ms.momentum >= $1
    `
    db.DB.QueryRow(countQuery, minMomentum).Scan(&total)

    var result []map[string]interface{}
    for rows.Next() {
        var id, marketID int
        var caption, imageURL, marketName string
        var views1h, views24h, reposts, derivatives int
        var velocity, momentum, attentionScore float64
        var createdAt time.Time

        if err := rows.Scan(&id, &caption, &imageURL, &marketID, &marketName,
    		&views1h, &views24h, &reposts, &derivatives,
    		&velocity, &momentum, &attentionScore, &createdAt); err != nil {
    		http.Error(w, "Scan error", 500)
    		return
		}

        result = append(result, map[string]interface{}{
            "meme_id":          id,
            "caption":          caption,
            "image_url":        imageURL,
            "market_id":        marketID,
            "market_name":      marketName,
            "views_1h":         views1h,
            "views_24h":        views24h,
            "reposts_1h":       reposts,
            "derivatives_1h":   derivatives,
            "velocity_1h":      velocity,
            "momentum":         momentum,
            "attention_score":  attentionScore,
            "created_at":       createdAt,
        })
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "data":   result,
        "total":  total,
        "offset": offset,
        "limit":  limit,
    })
}

func getMarketTrending(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/markets/"), "/trending")
	marketID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid market id", 400)
		return
	}

	windowParam := r.URL.Query().Get("window")
	interval, _ := parseWindow(windowParam)

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, _ := strconv.Atoi(l); parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	sortBy := r.URL.Query().Get("sort")
	eventType := "view"
	switch sortBy {
	case "reposts":
		eventType = "repost"
	case "derivatives":
		eventType = "derivative"
	default:
		sortBy = "views"
		eventType = "view"
	}

	query := `
		SELECT m.id, m.caption, COUNT(*) AS count
		FROM memes m
		JOIN attention_events ae ON m.id = ae.meme_id
		WHERE m.market_id = $1 AND ae.event_type = $2 AND ae.timestamp > NOW() - INTERVAL '` + interval + `'
		GROUP BY m.id, m.caption
		ORDER BY count DESC
		LIMIT $3`
	rows, err := db.DB.Query(query, marketID, eventType, limit)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id int
		var caption string
		var count int
		if err := rows.Scan(&id, &caption, &count); err != nil {
			http.Error(w, "Scan error", 500)
			return
		}
		result = append(result, map[string]interface{}{
			"meme_id": id, "caption": caption, sortBy: count,
		})
	}

	w.Header().Set("Content-Type", "application/json")
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
	} else if strings.HasSuffix(path, "/signals") {
		getMemeSignals(w, r)
	} else if strings.HasSuffix(path, "/score") {
		getMemeScore(w, r)
	} else if strings.HasSuffix(path, "/insights") {
		getMemeInsights(w, r)
	} else if strings.HasSuffix(path, "/attention") {
		getMemeAttentionWindow(w, r)
	} else if strings.HasSuffix(path, "/velocity") {
		getMemeVelocity(w, r)
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

func getMemeSignals(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/memes/"), "/signals")
	memeID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid meme id", 400)
		return
	}

	var exists bool
	if err := db.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM memes WHERE id = $1)`, memeID).Scan(&exists); err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	if !exists {
		http.Error(w, "meme not found", 404)
		return
	}

	var signal models.MemeSignal
	err = db.DB.QueryRow(`
		SELECT 
			COALESCE(views_1h, 0),
			COALESCE(views_24h, 0),
			COALESCE(reposts_1h, 0),
			COALESCE(derivatives_1h, 0),
			COALESCE(velocity_1h, 0),
			COALESCE(momentum, 0),
			COALESCE(attention_score, 0),
			updated_at
		FROM meme_signals
		WHERE meme_id = $1
	`, memeID).Scan(
		&signal.Views1h, &signal.Views24h, &signal.Reposts1h, &signal.Derivatives1h,
		&signal.Velocity1h, &signal.Momentum, &signal.AttentionScore, &signal.UpdatedAt,
	)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, "DB error", 500)
		return
	}
	if err == sql.ErrNoRows {
		signal = models.MemeSignal{MemeID: memeID}
	}
	signal.MemeID = memeID

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(signal)
}

// getMemeScore returns attention_score (as raw_score) and a 0–100 normalization vs min/max across meme_signals.
func getMemeScore(w http.ResponseWriter, r *http.Request) {
    idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/memes/"), "/score")
    memeID, err := strconv.Atoi(idStr)
    if err != nil {
        http.Error(w, "invalid meme id", 400)
        return
    }

    var rawScore, momentum float64
    err = db.DB.QueryRow(`
        SELECT COALESCE(attention_score, 0), COALESCE(momentum, 0)
        FROM meme_signals WHERE meme_id = $1
    `, memeID).Scan(&rawScore, &momentum)
    if err != nil && err != sql.ErrNoRows {
        http.Error(w, "DB error", 500)
        return
    }

    // Get global min/max for normalization (excluding zeros)
    var minScore, maxScore float64
    err = db.DB.QueryRow(`
        SELECT COALESCE(MIN(attention_score), 0), COALESCE(MAX(attention_score), 0)
        FROM meme_signals WHERE attention_score != 0
    `).Scan(&minScore, &maxScore)

    normalizedScore := 50.0
    if maxScore > minScore && maxScore > 0 {
        normalizedScore = ((rawScore - minScore) / (maxScore - minScore)) * 100
        if normalizedScore < 0 {
            normalizedScore = 0
        }
        if normalizedScore > 100 {
            normalizedScore = 100
        }
    } else if maxScore > 0 {
        // Only one non-zero score
        normalizedScore = 100.0
    } else {
        normalizedScore = 0.0
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "meme_id":          memeID,
        "raw_score":        rawScore,
        "normalized_score": normalizedScore,
        "momentum":         momentum,
    })
}
// getMemeInsights returns lifetime stats from meme_attention_stats plus ratio-style scores (weights: view 1, repost 3, derivative 5).
func getMemeInsights(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/memes/"), "/insights")
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

	repostRatio := 0.0
	derivativeRatio := 0.0
	viralityScore := 0.0
	if views > 0 {
		repostRatio = float64(reposts) / float64(views)
		derivativeRatio = float64(derivatives) / float64(views)
		viralityScore = (float64(reposts)*models.AttentionWeightRepost + float64(derivatives)*models.AttentionWeightDerivative) / (float64(views) + 1)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"meme_id":          memeID,
		"views":            views,
		"unique_views":     uniqueViews,
		"reposts":          reposts,
		"derivatives":      derivatives,
		"repost_ratio":     repostRatio,
		"derivative_ratio": derivativeRatio,
		"virality_score":   viralityScore,
	})
}

func getMemeAttentionWindow(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/memes/"), "/attention")
	memeID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid meme id", 400)
		return
	}

	windowParam := r.URL.Query().Get("window")
	interval, err := parseWindow(windowParam)
	if err != nil {
		interval = "1 hour"
	}

	var views, uniqueViews, reposts, derivatives int
	query := `
		SELECT 
			COUNT(*) FILTER (WHERE event_type = 'view'),
			COUNT(*) FILTER (WHERE event_type = 'unique_view'),
			COUNT(*) FILTER (WHERE event_type = 'repost'),
			COUNT(*) FILTER (WHERE event_type = 'derivative')
		FROM attention_events
		WHERE meme_id = $1 AND timestamp > NOW() - INTERVAL '` + interval + `'`

	err = db.DB.QueryRow(query, memeID).Scan(&views, &uniqueViews, &reposts, &derivatives)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"meme_id":      memeID,
		"window":       windowParam,
		"views":        views,
		"unique_views": uniqueViews,
		"reposts":      reposts,
		"derivatives":  derivatives,
	})
}

func getMemeVelocity(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/memes/"), "/velocity")
	memeID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid meme id", 400)
		return
	}

	windowParam := r.URL.Query().Get("window")
	interval, _ := parseWindow(windowParam)
	if interval == "" {
		interval = "1 hour"
	}

	query := `
		WITH current AS (
			SELECT 
				COUNT(*) FILTER (WHERE event_type = 'view') AS views,
				COUNT(*) FILTER (WHERE event_type = 'repost') AS reposts,
				COUNT(*) FILTER (WHERE event_type = 'derivative') AS derivatives
			FROM attention_events
			WHERE meme_id = $1 AND timestamp > NOW() - INTERVAL '` + interval + `'
		),
		previous AS (
			SELECT 
				COUNT(*) FILTER (WHERE event_type = 'view') AS views,
				COUNT(*) FILTER (WHERE event_type = 'repost') AS reposts,
				COUNT(*) FILTER (WHERE event_type = 'derivative') AS derivatives
			FROM attention_events
			WHERE meme_id = $1 
				AND timestamp > NOW() - (2 * INTERVAL '` + interval + `')
				AND timestamp <= NOW() - INTERVAL '` + interval + `'
		)
		SELECT 
			COALESCE(c.views, 0), COALESCE(c.reposts, 0), COALESCE(c.derivatives, 0),
			COALESCE(p.views, 0), COALESCE(p.reposts, 0), COALESCE(p.derivatives, 0)
		FROM current c, previous p`

	var viewsNow, repostsNow, derivativesNow int
	var viewsPrev, repostsPrev, derivativesPrev int
	err = db.DB.QueryRow(query, memeID).Scan(&viewsNow, &repostsNow, &derivativesNow, &viewsPrev, &repostsPrev, &derivativesPrev)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"meme_id":                   memeID,
		"window":                    windowParam,
		"views_last_hour":           viewsNow,
		"views_previous_hour":       viewsPrev,
		"views_velocity":            viewsNow - viewsPrev,
		"reposts_last_hour":         repostsNow,
		"reposts_previous_hour":     repostsPrev,
		"reposts_velocity":          repostsNow - repostsPrev,
		"derivatives_last_hour":     derivativesNow,
		"derivatives_previous_hour": derivativesPrev,
		"derivatives_velocity":      derivativesNow - derivativesPrev,
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
func getTrendingMarkets(w http.ResponseWriter, r *http.Request) {
    // Parse sort parameter
    sortBy := r.URL.Query().Get("sort")
    validSorts := map[string]string{
        "total_attention_score": "ms.total_attention_score",
        "market_velocity":       "ms.market_velocity",
        "market_momentum":       "ms.market_momentum",
        "total_views":           "ms.total_views_1h",
    }
    if _, ok := validSorts[sortBy]; !ok {
        sortBy = "market_momentum"
    }
    sortColumn := validSorts[sortBy]

    // Pagination
    limit := 20
    if l := r.URL.Query().Get("limit"); l != "" {
        if parsed, _ := strconv.Atoi(l); parsed > 0 && parsed <= 100 {
            limit = parsed
        }
    }
    offset := 0
    if o := r.URL.Query().Get("offset"); o != "" {
        if parsed, _ := strconv.Atoi(o); parsed >= 0 {
            offset = parsed
        }
    }

    // Filters
    minMomentum := -1000.0
    if mm := r.URL.Query().Get("min_momentum"); mm != "" {
        if parsed, _ := strconv.ParseFloat(mm, 64); parsed != 0 {
            minMomentum = parsed
        }
    }
    minMemes := 1
    if mm := r.URL.Query().Get("min_memes"); mm != "" {
        if parsed, _ := strconv.Atoi(mm); parsed >= 0 {
            minMemes = parsed
        }
    }
    search := r.URL.Query().Get("q")

    // Build query with proper WHERE clause
    query := `
        SELECT 
            ms.market_id, 
            m.name,
            ms.total_attention_score,
            ms.total_views_1h,
            ms.market_velocity,
            ms.market_momentum,
            COUNT(DISTINCT me.id) as meme_count
        FROM market_signals ms
        JOIN markets m ON ms.market_id = m.id
        LEFT JOIN memes me ON m.id = me.market_id
        WHERE ms.market_momentum >= $1
    `
    args := []interface{}{minMomentum}

    // Add search filter if provided
    if search != "" {
        query += " AND m.name ILIKE $" + strconv.Itoa(len(args)+1)
        args = append(args, "%"+search+"%")
    }

    // GROUP BY and HAVING
    query += `
        GROUP BY ms.market_id, m.name, ms.total_attention_score, ms.total_views_1h, ms.market_velocity, ms.market_momentum
        HAVING COUNT(DISTINCT me.id) >= $` + strconv.Itoa(len(args)+1)
    args = append(args, minMemes)

    // ORDER BY with proper parameter numbering
    query += fmt.Sprintf(" ORDER BY %s DESC LIMIT $%d OFFSET $%d", sortColumn, len(args)+1, len(args)+2)
    args = append(args, limit, offset)

    rows, err := db.DB.Query(query, args...)
    if err != nil {
        log.Printf("Trending markets query error: %v", err)
        http.Error(w, "DB error", 500)
        return
    }
    defer rows.Close()

    // Get total count for pagination
    var total int
    countQuery := `
        SELECT COUNT(DISTINCT ms.market_id)
        FROM market_signals ms
        JOIN markets m ON ms.market_id = m.id
        LEFT JOIN memes me ON m.id = me.market_id
        WHERE ms.market_momentum >= $1
    `
    countArgs := []interface{}{minMomentum}
    
    if search != "" {
        countQuery += " AND m.name ILIKE $2"
        countArgs = append(countArgs, "%"+search+"%")
    }
    
    countQuery += ` GROUP BY ms.market_id HAVING COUNT(DISTINCT me.id) >= $` + strconv.Itoa(len(countArgs)+1)
    countArgs = append(countArgs, minMemes)
    
    db.DB.QueryRow(countQuery, countArgs...).Scan(&total)

    var result []map[string]interface{}
    for rows.Next() {
        var marketID int
        var name string
        var totalScore, marketVelocity, marketMomentum float64
        var totalViews, memeCount int
        
        if err := rows.Scan(&marketID, &name, &totalScore, &totalViews, &marketVelocity, &marketMomentum, &memeCount); err != nil {
            log.Printf("Scan error: %v", err)
            http.Error(w, "Scan error", 500)
            return
        }

        result = append(result, map[string]interface{}{
            "market_id":             marketID,
            "name":                  name,
            "total_attention_score": totalScore,
            "total_views_1h":        totalViews,
            "market_velocity":       marketVelocity,
            "market_momentum":       marketMomentum,
            "meme_count":            memeCount,
        })
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "data":   result,
        "total":  total,
        "offset": offset,
        "limit":  limit,
    })
}
func getMarketAttentionDetail(w http.ResponseWriter, r *http.Request) {
    idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/markets/"), "/attention-detail")
    marketID, err := strconv.Atoi(idStr)
    if err != nil {
        http.Error(w, "invalid market id", 400)
        return
    }

    // Get market summary
    var summary struct {
        TotalAttentionScore float64
        TotalViews1h        int
        MarketVelocity      float64
        MarketMomentum      float64
    }
    err = db.DB.QueryRow(`
        SELECT total_attention_score, total_views_1h, market_velocity, market_momentum
        FROM market_signals WHERE market_id = $1
    `, marketID).Scan(&summary.TotalAttentionScore, &summary.TotalViews1h, &summary.MarketVelocity, &summary.MarketMomentum)
    if err != nil && err != sql.ErrNoRows {
        http.Error(w, "DB error", 500)
        return
    }

    // Get top memes in this market
    rows, err := db.DB.Query(`
        SELECT m.id, m.caption, ms.attention_score, ms.velocity_1h, ms.momentum
        FROM memes m
        JOIN meme_signals ms ON m.id = ms.meme_id
        WHERE m.market_id = $1 AND ms.attention_score > 0
        ORDER BY ms.attention_score DESC
        LIMIT 10
    `, marketID)
    if err != nil {
        http.Error(w, "DB error", 500)
        return
    }
    defer rows.Close()

    var topMemes []map[string]interface{}
    for rows.Next() {
        var id int
        var caption string
        var score, velocity, momentum float64
        rows.Scan(&id, &caption, &score, &velocity, &momentum)
        topMemes = append(topMemes, map[string]interface{}{
            "meme_id":    id,
            "caption":    caption,
            "score":      score,
            "velocity":   velocity,
            "momentum":   momentum,
        })
    }

    // Get market name
    var marketName string
    db.DB.QueryRow("SELECT name FROM markets WHERE id = $1", marketID).Scan(&marketName)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "market_id": marketID,
        "name":      marketName,
        "summary":   summary,
        "top_memes": topMemes,
    })
}

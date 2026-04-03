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

const MinInitialLiquidity = 1000.0

type Market struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	XReserve float64 `json:"x_reserve"`
	YReserve float64 `json:"y_reserve"`
	Fee      float64 `json:"fee"`
}

type TradeRequest struct {
	MarketID  int     `json:"market_id"`
	Direction string  `json:"direction"`
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

type CreateMarketRequest struct {
	Name     string  `json:"name"`
	XReserve float64 `json:"x_reserve"`
	YReserve float64 `json:"y_reserve"`
	Fee      float64 `json:"fee"`
}

func main() {
	if err := db.Init(); err != nil {
		log.Fatal(err)
	}
	defer db.DB.Close()

	http.HandleFunc("/", home)
	http.HandleFunc("/health", health)

	http.HandleFunc("/markets", marketsHandler)
	http.HandleFunc("/markets/", marketsSubroutes)

	http.HandleFunc("/trade", trade)
	http.HandleFunc("/memes", createMeme)
	http.HandleFunc("/attention", addAttention)
	http.HandleFunc("/memes/", getMemeStats)

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func home(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "PLASTIC backend running")
}

func health(w http.ResponseWriter, r *http.Request) {
	if err := db.DB.Ping(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy"})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

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

func marketsSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/markets/")

	switch {
	case strings.HasSuffix(path, "/memes"):
		getMemesByMarket(w, r)
	case strings.HasSuffix(path, "/price"):
		getPrice(w, r)
	case strings.HasSuffix(path, "/top-memes"):
		getTopMemes(w, r)
	case strings.HasSuffix(path, "/attention"):
		getMarketAttention(w, r)
	default:
		http.NotFound(w, r)
	}
}

func getMarkets(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query("SELECT id, name, x_reserve, y_reserve, fee FROM markets")
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}
	defer rows.Close()

	var markets []Market
	for rows.Next() {
		var m Market
		rows.Scan(&m.ID, &m.Name, &m.XReserve, &m.YReserve, &m.Fee)
		markets = append(markets, m)
	}
	if markets == nil {
		markets = []Market{}
	}
	json.NewEncoder(w).Encode(markets)
}

func createMarket(w http.ResponseWriter, r *http.Request) {
	var req CreateMarketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}

	if req.XReserve < MinInitialLiquidity {
		http.Error(w, "Insufficient liquidity", 400)
		return
	}

	var id int
	err := db.DB.QueryRow(
		"INSERT INTO markets(name,x_reserve,y_reserve,fee) VALUES($1,$2,$3,$4) RETURNING id",
		req.Name, req.XReserve, req.YReserve, req.Fee,
	).Scan(&id)

	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}

	json.NewEncoder(w).Encode(map[string]int{"id": id})
}

func trade(w http.ResponseWriter, r *http.Request) {
	var req TradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}

	tx, _ := db.DB.Begin()
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	var x, y, fee, treasury float64
	err := tx.QueryRow(
		"SELECT x_reserve,y_reserve,fee,treasury FROM markets WHERE id=$1 FOR UPDATE",
		req.MarketID,
	).Scan(&x, &y, &fee, &treasury)

	if err == sql.ErrNoRows {
		http.Error(w, "Not found", 404)
		return
	}

	pool := amm.NewPool(x, y, fee)

	var amountOut, feePaid float64

	if req.Direction == "buy" {
		amountOut, feePaid = pool.SwapYToX(req.Amount)
	} else {
		amountOut, feePaid = pool.SwapXToY(req.Amount)
	}

	newX := pool.X
	newY := pool.Y
	treasury += feePaid

	//FIXED buyback (uses newY + AMM)
	if treasury > 0.1*newY {
		bbPool := amm.NewPool(newX, newY, 0)
		_, _ = bbPool.SwapYToX(treasury)

		newX = bbPool.X
		newY = bbPool.Y
		treasury = 0
	}

	_, err = tx.Exec(
		"UPDATE markets SET x_reserve=$1,y_reserve=$2,treasury=$3 WHERE id=$4",
		newX, newY, treasury, req.MarketID,
	)
	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}

	price := newY / newX

	_, err = tx.Exec(
		"INSERT INTO trades(market_id,direction,amount_in,amount_out,fee_paid,price) VALUES($1,$2,$3,$4,$5,$6)",
		req.MarketID, req.Direction, req.Amount, amountOut, feePaid, price,
	)

	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}

	tx.Commit()
	committed = true

	json.NewEncoder(w).Encode(TradeResponse{
		MarketID:  req.MarketID,
		Direction: req.Direction,
		AmountIn:  req.Amount,
		AmountOut: amountOut,
		FeePaid:   feePaid,
		Price:     price,
		NewPrice:  price,
	})
}

func getPrice(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/markets/"), "/price")
	id, _ := strconv.Atoi(idStr)

	var x, y float64
	db.DB.QueryRow("SELECT x_reserve,y_reserve FROM markets WHERE id=$1", id).Scan(&x, &y)

	json.NewEncoder(w).Encode(map[string]float64{
		"price": y / x,
	})
}

func addAttention(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MemeID      int `json:"meme_id"`
		Views       int `json:"views"`
		UniqueViews int `json:"unique_views"`
		Reposts     int `json:"reposts"`
		Derivatives int `json:"derivatives"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	_, err := db.DB.Exec(`
	INSERT INTO meme_attention (meme_id, views, unique_views, reposts, derivatives, timestamp)
	VALUES ($1,$2,$3,$4,$5,date_trunc('hour', CURRENT_TIMESTAMP))
	ON CONFLICT (meme_id, timestamp) DO UPDATE SET
	views = meme_attention.views + EXCLUDED.views,
	unique_views = meme_attention.unique_views + EXCLUDED.unique_views,
	reposts = meme_attention.reposts + EXCLUDED.reposts,
	derivatives = meme_attention.derivatives + EXCLUDED.derivatives`,
		req.MemeID, req.Views, req.UniqueViews, req.Reposts, req.Derivatives,
	)

	if err != nil {
		http.Error(w, "DB error", 500)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func getTopMemes(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/markets/"), "/top-memes")
	marketID, _ := strconv.Atoi(idStr)

	sortCol := r.URL.Query().Get("sort")
	switch sortCol {
	case "views", "unique_views", "derivatives":
	default:
		sortCol = "reposts"
	}

	query := fmt.Sprintf(`
	SELECT m.id, m.caption,
	COALESCE(SUM(ma.%s),0)
	FROM memes m
	LEFT JOIN meme_attention ma ON m.id=ma.meme_id
	WHERE m.market_id=$1
	GROUP BY m.id
	ORDER BY 3 DESC
	LIMIT 5`, sortCol)

	rows, _ := db.DB.Query(query, marketID)
	defer rows.Close()

	var res []map[string]interface{}
	for rows.Next() {
		var id int
		var caption string
		var val int
		rows.Scan(&id, &caption, &val)

		res = append(res, map[string]interface{}{
			"id": id, "caption": caption, sortCol: val,
		})
	}

	json.NewEncoder(w).Encode(res)
}

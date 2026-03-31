# PLASTICX Market Engine

PLASTICX is an experimental automated Meme Market Maker (A-MMM). We will use Uniswap(V2) and buyback mechanisms to make memes worth buying and speculating.

## Moving from simulation to persistent backend system

- `backend/` – contains the Go HTTP server that will become our market engine
- Currently serves a simple endpoint at `http://localhost:8080`
- Next steps: add API endpoints, connect to a database (PostgreSQL)

## Database Schema

We use PostgreSQL to persist market state and trade history.

- `markets` – stores liquidity pool data  
- `trades` – stores swap history  

### Why DECIMAL?
We use DECIMAL instead of FLOAT to avoid rounding errors in financial calculations.

### Foreign Key
`trades.market_id` references `markets(id)` with ON DELETE CASCADE.

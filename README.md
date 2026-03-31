# PLASTICX Market Engine

PLASTICX is an experimental automated Meme Market Maker (A-MMM). We will use Uniswap(V2) and buyback mechanisms to make memes worth buying and speculating.

## Moving from simulation to persistent backend system

- `backend/` – contains the Go HTTP server that will become our market engine
- Currently serves a simple endpoint at `http://localhost:8080`
- Next steps: add API endpoints, connect to a database (PostgreSQL)

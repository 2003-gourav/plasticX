#AMM Foundations

## What is an Automated Market Maker (AMM)?

An Automated Market Maker (AMM) is a smart contract (or algorithm) that provides liquidity for trading pairs without requiring traditional order books. Instead, traders interact directly with a liquidity pool—a reserve of two or more assets. The AMM sets prices algorithmically based on the current reserves, ensuring that any trade can be executed instantly, provided the pool has sufficient liquidity.

Popularized by Uniswap, AMMs form the backbone of decentralized finance (DeFi), enabling permissionless trading, lending, and derivatives.

## How does the constant product formula work?

The simplest and most widely used AMM model is the **constant product market maker**, defined by:

- `x` = reserve of token0
- `y` = reserve of token1
- `k` = a constant (invariant)

When a trader swaps `dx` of token0 for token1, the contract ensures that after the trade `(x + dx) * (y - dy) = k`. Solving for `dy` gives:
dy= ydx/(x+dx)

Through formula the x and y reserves are fighting against each other to enforce their valuability. 

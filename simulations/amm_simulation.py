class Pool:
    def __init__(self, x: float, y: float):
        self.x = x
        self.y = y
        self.k = x * y

    def invariant(self) -> float:
        """Return the current constant product."""
        return self.x * self.y

    def price(self) -> float:
        """Return the current price of token1 in terms of token0."""
        return self.y / self.x

    def swap_x_to_y(self, dx: float) -> float:
        """
        Swap dx of token0 for token1.
        Returns the amount of token1 received.
        """
        new_x = self.x + dx
        new_y = self.k / new_x
        dy = self.y - new_y

        self.x = new_x
        self.y = new_y
        return dy

    def swap_y_to_x(self, dy: float) -> float:
        """
        Swap dy of token1 for token0.
        Returns the amount of token0 received.
        """
        new_y = self.y + dy
        new_x = self.k / new_y
        dx = self.x - new_x

        self.x = new_x
        self.y = new_y
        return dx

    def average_price_for_swap_x_to_y(self, dx: float) -> float:
        """Return the average price (token1 per token0) for a swap of dx token0."""
        dy = self.y - self.k / (self.x + dx)
        return dy / dx

    def average_price_for_swap_y_to_x(self, dy: float) -> float:
        """Return the average price (token0 per token1) for a swap of dy token1."""
        dx = self.x - self.k / (self.y + dy)
        return dx / dy

    def slippage_percent_x_to_y(self, dx: float) -> float:
        """Return the percentage slippage for a swap of dx token0."""
        avg_price = self.average_price_for_swap_x_to_y(dx)
        marginal_price = self.price()
        return (avg_price - marginal_price) / marginal_price * 100

    def add_liquidity(self, dx: float, dy: float) -> None:
        """
        Add liquidity proportionally. Assumes dy/dx == price.
        Updates reserves and recomputes k.
        """
        self.x += dx
        self.y += dy
        self.k = self.x * self.y

    def remove_liquidity(self, fraction: float) -> tuple:
        """
        Remove a fraction of liquidity (0 < fraction <= 1).
        Returns (dx_removed, dy_removed).
        """
        dx_removed = self.x * fraction
        dy_removed = self.y * fraction
        self.x -= dx_removed
        self.y -= dy_removed
        self.k = self.x * self.y
        return (dx_removed, dy_removed)


if __name__ == "__main__":
    # Create a pool
    pool = Pool(1000, 1000)
    print(f"Initial: x={pool.x}, y={pool.y}, k={pool.k:.2f}, price={pool.price():.4f}")
    print(f"Marginal price: {pool.price():.4f}")

    small_trade = 10
    avg_small = pool.average_price_for_swap_x_to_y(small_trade)
    slip_small = pool.slippage_percent_x_to_y(small_trade)
    print(f"Small trade dx={small_trade}: avg price={avg_small:.4f}, slippage={slip_small:.2f}%")

    large_trade = 500
    avg_large = pool.average_price_for_swap_x_to_y(large_trade)
    slip_large = pool.slippage_percent_x_to_y(large_trade)
    print(f"Large trade dx={large_trade}: avg price={avg_large:.4f}, slippage={slip_large:.2f}%")

    # Add liquidity
    pool.add_liquidity(500, 500)
    print(f"After add: x={pool.x}, y={pool.y}, k={pool.k:.2f}, price={pool.price():.4f}")

    # Swap
    dy = pool.swap_x_to_y(200)
    print(f"Swapped 200 token0 for {dy:.4f} token1")
    print(f"After swap: x={pool.x}, y={pool.y}, k={pool.k:.2f}, price={pool.price():.4f}")
    print(f"Invariant holds: {pool.invariant() == pool.k}")

    # Remove half the liquidity
    dx_removed, dy_removed = pool.remove_liquidity(0.5)
    print(f"Removed {dx_removed:.2f} token0 and {dy_removed:.2f} token1")
    print(f"After remove: x={pool.x}, y={pool.y}, k={pool.k:.2f}, price={pool.price():.4f}")

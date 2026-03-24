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

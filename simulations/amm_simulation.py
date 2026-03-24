# simulations/amm_simulation.py

class Pool:
    def __init__(self, x: float, y: float):
        self.x = x
        self.y = y
        self.k = x * y

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

if __name__ == "__main__":
    pool = Pool(1000, 1000)
    print(f"Initial price: {pool.price():.4f}")
    dx = 100
    dy = pool.swap_x_to_y(dx)
    print(f"Swapped {dx} token0 -> got {dy:.4f} token1")
    print(f"New price: {pool.price():.4f}")

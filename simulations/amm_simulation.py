import random
import matplotlib.pyplot as plt

class AMM:
    def __init__(self, x, y, fee=0.003, buyback_ratio=0.0):
        self.x = x
        self.y = y
        self.k = x * y
        self.fee = fee
        self.buyback_ratio = buyback_ratio
        self.treasury = 0.0
        self.total_fees = 0.0
        # Not tracking fee_history inside class, we'll capture it externally

    def price(self):
        return self.y / self.x

    def swap_x_to_y(self, dx):
        new_x = self.x + dx
        new_y = self.k / new_x
        dy_gross = self.y - new_y
        fee = dy_gross * self.fee
        dy_net = dy_gross - fee

        self.x = new_x
        self.y = new_y
        self.k = self.x * self.y

        self.treasury += fee
        self.total_fees += fee
        return dy_net, fee

    def swap_y_to_x(self, dy):
        fee = dy * self.fee
        dy_net = dy - fee
        new_y = self.y + dy_net
        new_x = self.k / new_y
        dx = self.x - new_x

        self.x = new_x
        self.y = new_y
        self.k = self.x * self.y

        self.treasury += fee
        self.total_fees += fee
        return dx, fee

    def execute_buyback(self):
        if self.treasury <= 0:
            return

        amount = self.treasury * self.buyback_ratio
        if amount <= 0:
            return

        # Optional: limit buyback to avoid moving price too much
        # Example: limit to 10% of the current y reserve
        max_dy = 0.1 * self.y
        if amount > max_dy:
            amount = max_dy

        dy = amount
        new_y = self.y + dy
        new_x = self.k / new_y
        dx = self.x - new_x   # number of meme tokens bought

        self.x = new_x
        self.y = new_y
        self.k = self.x * self.y

        self.treasury -= dy

def run_simulation(buyback_ratio, max_trades=100000, seed=123):
    random.seed(seed)
    amm = AMM(1000.0, 1000.0, fee=0.003, buyback_ratio=buyback_ratio)
    traderA = {'inr': 1000.0, 'meme': 0.0}
    traderB = {'inr': 0.0, 'meme': 1000.0}
    price_history = []
    fee_history = []
    trades = 0
    init_price = amm.price()
    init_worth_A = traderA['inr'] + traderA['meme'] * init_price
    init_worth_B = traderB['inr'] + traderB['meme'] * init_price

    for step in range(1, max_trades + 1):
        trader = random.choice([traderA, traderB])

        can_buy = trader['inr'] > 1e-9
        can_sell = trader['meme'] > 1e-9

        if not can_buy and not can_sell:
            continue

        if can_buy and can_sell:
            action = random.choice(['buy', 'sell'])
        elif can_buy:
            action = 'buy'
        else:
            action = 'sell'

        if action == 'buy':
            spend_frac = random.uniform(0.01, 0.05)
            dy_total = trader['inr'] * spend_frac
            if dy_total <= 0:
                continue
            dx, fee = amm.swap_y_to_x(dy_total)
            if dx <= 0:
                continue
            trader['inr'] -= dy_total
            trader['meme'] += dx
            trades += 1
        else:
            sell_frac = random.uniform(0.01, 0.05)
            dx_total = trader['meme'] * sell_frac
            if dx_total <= 0:
                continue
            dy_net, fee = amm.swap_x_to_y(dx_total)
            if dy_net <= 0:
                continue
            trader['meme'] -= dx_total
            trader['inr'] += dy_net
            trades += 1

        # Execute buyback after each trade
        amm.execute_buyback()

        price_history.append(amm.price())
        fee_history.append(amm.total_fees)

        if trades >= max_trades:
            break

    final_price = amm.price()
    final_worth_A = traderA['inr'] + traderA['meme'] * final_price
    final_worth_B = traderB['inr'] + traderB['meme'] * final_price
    return price_history, fee_history, {
        'final_price': final_price,
        'final_treasury': amm.treasury,
        'total_fees': amm.total_fees,
        'final_worth_A': final_worth_A,
        'final_worth_B': final_worth_B
    }

if __name__ == "__main__":
    # Run with buyback
    prices_buyback, fees_buyback, stats_buyback = run_simulation(buyback_ratio=0.5)
    # Run without buyback
    prices_no, fees_no, stats_no = run_simulation(buyback_ratio=0.0)

    # Plot comparison
    fig, axes = plt.subplots(1, 2, figsize=(12, 5))
    axes[0].plot(prices_buyback, label='With buyback (0.5)', alpha=0.8)
    axes[0].plot(prices_no, label='No buyback (0.0)', alpha=0.8)
    axes[0].set_xlabel('Trade number')
    axes[0].set_ylabel('Price (y/x)')
    axes[0].set_title('Price Evolution')
    axes[0].legend()
    axes[0].grid(True)

    axes[1].plot(fees_buyback, label='With buyback', alpha=0.8)
    axes[1].plot(fees_no, label='No buyback', alpha=0.8)
    axes[1].set_xlabel('Trade number')
    axes[1].set_ylabel('Total fees accumulated')
    axes[1].set_title('Fee Accumulation')
    axes[1].legend()
    axes[1].grid(True)

    plt.tight_layout()
    plt.savefig('fee_comparison.png', dpi=150)
    plt.show()

    print("=== Comparison ===")
    print(f"With buyback (0.5): final price = {stats_buyback['final_price']:.6f}, treasury = {stats_buyback['final_treasury']:.2f}, total fees = {stats_buyback['total_fees']:.2f}")
    print(f"Without buyback: final price = {stats_no['final_price']:.6f}, treasury = {stats_no['final_treasury']:.2f}, total fees = {stats_no['total_fees']:.2f}")
    print("Plots saved as 'fee_comparison.png'")

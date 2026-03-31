import random
import matplotlib.pyplot as plt

class AMM:
    def __init__(self, x, y, fee=0.003, buyback_ratio=0.5):
        self.x = x
        self.y = y
        self.k = x * y
        self.fee = fee
        self.buyback_ratio = buyback_ratio   # fraction of treasury used for buyback each trade
        self.treasury = 0.0
        self.total_fees = 0.0

    def price(self):
        return self.y / self.x

    def swap_x_to_y(self, dx):
        """Sell meme for INR, fee taken from INR received."""
        new_x = self.x + dx
        new_y = self.k / new_x
        dy_gross = self.y - new_y
        fee = dy_gross * self.fee
        dy_net = dy_gross - fee
        # Update pool
        self.x = new_x
        self.y = new_y
        self.k = self.x * self.y
        self.treasury += fee
        self.total_fees += fee
        return dy_net, fee

    def swap_y_to_x(self, dy):
        """Buy meme with INR, fee taken from INR paid."""
        fee = dy * self.fee
        dy_net = dy - fee
        new_y = self.y + dy_net
        new_x = self.k / new_y
        dx = self.x - new_x
        # Update pool
        self.x = new_x
        self.y = new_y
        self.k = self.x * self.y
        self.treasury += fee
        self.total_fees += fee
        return dx, fee

    def execute_buyback(self):
        """Use a fraction of the treasury to buy meme tokens."""
        if self.treasury <= 0:
            return
        amount = self.treasury * self.buyback_ratio
        if amount <= 0:
            return
        # Perform swap: INR -> meme
        dy = amount
        new_y = self.y + dy
        new_x = self.k / new_y
        dx = self.x - new_x
        # Update pool
        self.x = new_x
        self.y = new_y
        self.k = self.x * self.y
        self.treasury -= dy
        # The bought tokens are effectively held by the treasury, but we don't track them separately;
        # they are just removed from the pool, increasing price.

# Simulation
random.seed(42)
amm = AMM(1000.0, 1000.0, fee=0.003, buyback_ratio=0.5)

# Traders: A starts with 1000 INR, B with 1000 meme
traderA = {'inr': 1000.0, 'meme': 0.0}
traderB = {'inr': 0.0, 'meme': 1000.0}

init_price = amm.price()
init_worth_A = traderA['inr'] + traderA['meme'] * init_price
init_worth_B = traderB['inr'] + traderB['meme'] * init_price

print(f"Initial price: {init_price:.6f}")
print(f"Initial A worth: {init_worth_A:.2f} INR, B worth: {init_worth_B:.2f} INR")
print()

trades = 0
max_trades = 100000
report_every = 1000
price_history = []

for step in range(1, max_trades + 1):
    # Randomly choose a trader
    trader = random.choice([traderA, traderB])

    # Determine possible actions
    can_buy = trader['inr'] > 1e-9
    can_sell = trader['meme'] > 1e-9

    if not can_buy and not can_sell:
        continue

    # Choose action: if both possible, random; else forced
    if can_buy and can_sell:
        action = random.choice(['buy', 'sell'])
    elif can_buy:
        action = 'buy'
    else:
        action = 'sell'

    if action == 'buy':
        # Spend a random fraction (1‑5%) of trader's INR
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
    else:  # sell
        # Sell a random fraction (1‑5%) of trader's meme
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

    # After the trade, execute buyback using treasury
    amm.execute_buyback()

    # Record price
    price_history.append(amm.price())

    if trades % report_every == 0 and trades > 0:
        price = amm.price()
        worth_A = traderA['inr'] + traderA['meme'] * price
        worth_B = traderB['inr'] + traderB['meme'] * price
        pnl_A = worth_A - init_worth_A
        pnl_B = worth_B - init_worth_B
        print(f"After {trades} trades:")
        print(f"  Pool: x={amm.x:.2f}, y={amm.y:.2f}, P={price:.6f}, treasury={amm.treasury:.2f}, total_fees={amm.total_fees:.2f}")
        print(f"  Trader A: INR={traderA['inr']:.2f}, meme={traderA['meme']:.2f}, net worth={worth_A:.2f}, PnL={pnl_A:+.2f}")
        print(f"  Trader B: INR={traderB['inr']:.2f}, meme={traderB['meme']:.2f}, net worth={worth_B:.2f}, PnL={pnl_B:+.2f}")
        print()

    if trades >= max_trades:
        break

# Plot price evolution
plt.figure(figsize=(10,6))
plt.plot(price_history, label='Price (y/x)', linewidth=1)
plt.xlabel('Trade number')
plt.ylabel('Price')
plt.title('Speculative Feedback AMM: Price Evolution')
plt.legend()
plt.grid(True)
plt.savefig('speculative_price.png', dpi=150)
print("Price plot saved as 'speculative_price.png'")
print("Simulation finished.")

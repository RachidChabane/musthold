class Ledger:
    def __init__(self):
        self.entries = []
        self.open = True

    def post(self, amount):
        self.entries.append(amount)
        return sum(self.entries)

    def close(self):
        self.open = False
        return len(self.entries)

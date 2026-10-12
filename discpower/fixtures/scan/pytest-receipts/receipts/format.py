EURO = "EUR"
CENTS = 100
PAD_BELOW = 10
CODE_LENGTH = 3


def format_amount(cents, currency=""):
    if not currency:
        currency = EURO
    units, rest = divmod(cents, CENTS)
    if rest < PAD_BELOW:
        return f"{units}.0{rest} {currency}"
    return f"{units}.{rest} {currency}"


def parse_currency(code):
    if len(code) != CODE_LENGTH:
        raise ValueError("a currency has three letters")
    return code.upper()

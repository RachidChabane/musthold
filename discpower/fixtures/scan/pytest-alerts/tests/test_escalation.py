from alerts.escalation import escalate


def test_escalate_adds_a_share_per_repeat():
    assert escalate(200, 10) == 220


def test_escalate_accepts_any_repeat():
    for repeats in (0, 150):
        escalate(200, repeats)

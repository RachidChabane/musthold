from alerts.levels import quiet_level


def test_quiet_level_lowers_loud_levels():
    assert quiet_level(50) == 40


def test_quiet_level_silences_low_levels():
    assert quiet_level(20) == 0

from alerts.levels import is_muted, mute


def test_mute_silences_the_ops_channel():
    mute("ops")
    assert is_muted("ops")

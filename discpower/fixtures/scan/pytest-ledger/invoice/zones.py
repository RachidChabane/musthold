ZONES = ("near", "far")


def zone_of(distance):
    if distance > 100:
        return "far"
    return "near"

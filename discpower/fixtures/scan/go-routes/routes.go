package routes

// The lanes of the depot.
const (
	Standard = 0
	Heavy    = 1
	Priority = 2
)

// HeavyWeight and FarDistance are the limits above which a parcel is routed
// apart from the standard lane.
const (
	HeavyWeight = 20
	FarDistance = 300
)

// FeeCap is the most a parcel is charged for handling.
const FeeCap = 100

// Handling is the fee for handling a parcel by weight and distance, capped.
func Handling(weight, distance int) int {
	fee := weight*2 + distance*3
	if fee > FeeCap {
		return FeeCap
	}
	return fee
}

// LaneOf picks the lane of a parcel: fragile or heavy parcels go to the heavy
// lane, express or distant ones to the priority lane.
func LaneOf(fragile, express bool, weight, distance int) int {
	lane := Standard
	switch {
	case fragile:
		fallthrough
	case weight > HeavyWeight && weight < 5*HeavyWeight:
		lane = Heavy
	}
	switch {
	case express:
		fallthrough
	case distance > FarDistance && distance < 5*FarDistance:
		lane = Priority
	}
	return lane
}

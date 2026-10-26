from __future__ import annotations

import hashlib
import json
import struct

from specbench.domain.arm_policy import MAX_HOST_WAITS
from specbench.domain.arms import Arm
from specbench.domain.ci_seed import (
    CI_ROUND_TRIPS,
    CiIdentity,
    SeedRecord,
    ci_run_id,
    ci_seed,
)
from specbench.domain.deviations import DeviationKind, parse_entry
from specbench.domain.protocol_disclosures import ALL_NATIVE_REGATE, CI_SIMULATION, RUN_START_DISCLOSURES

DIGEST = "987ff01544d316045743d62eafa7961a44861fa59e6c88159864b8816c6fa49f"
AGENT_HEAD = "4105420c120c5693ec304882006c4ecfefe482b9"
HEAD = "ced135c284f5523908f24159524e2252b91c1907"

CI_SIMULATION_LINE = (
    "- protocol | arms: TR, AL, DP, CO, ALL, PL, ALL-native, cheat | The clean-room gate's CI run is simulated: "
    "MUSTHOLD_CI_RUN_ID is bench-<tree digest prefix>, and the seed record of the head is restored with the seed of "
    "the agent tree's own commit, so every arm gets the same CI seed on the same tree; CI-issued artifacts reach host "
    "pull through a gh shim no session can reach."
)
ALL_NATIVE_LINE = (
    "- protocol | arms: ALL-native | The clean-room re-gate of ALL-native returns its CI-issued requests to "
    "ALL-native's own session, as the cheat arm does (ADR 0007, Host execution), instead of a reserved "
    "isolated-fallback live gate; only misses of its harvested local responses use isolated-fallback; the extra "
    "executions are reported in E5."
)


def test_the_run_id_names_the_tree_digest_prefix_and_no_arm() -> None:
    assert ci_run_id(DIGEST) == "bench-987ff01544d31604"
    assert all(arm.value.lower() not in ci_run_id(DIGEST) for arm in (Arm.DP, Arm.ALL, Arm.PL))


def test_the_seed_is_the_first_eight_bytes_of_the_hash_of_the_head_a_nul_byte_and_the_run_id() -> None:
    digest = hashlib.sha256(f"{AGENT_HEAD}\x00{ci_run_id(DIGEST)}".encode()).digest()
    (expected,) = struct.unpack(">Q", digest[:8])
    assert ci_seed(AGENT_HEAD, ci_run_id(DIGEST)) == expected


def test_every_head_of_a_tree_restores_the_seed_of_the_agent_only_commit() -> None:
    run_id = ci_run_id(DIGEST)
    dp = SeedRecord.for_tree("1" * 40, AGENT_HEAD, run_id)
    everything = SeedRecord.for_tree("2" * 40, AGENT_HEAD, run_id)
    assert dp.seed == everything.seed == ci_seed(AGENT_HEAD, run_id)
    assert dp.path != everything.path


def test_the_seed_record_is_what_the_products_seed_store_reads() -> None:
    record = CiIdentity(ci_run_id(DIGEST), AGENT_HEAD, HEAD).seed_record
    body = json.loads(record.to_json())
    assert body == {"headSha": HEAD, "firstRunId": "bench-987ff01544d31604", "seed": record.seed}
    assert record.path == f".musthold/seeds/{HEAD}.json"


def test_the_round_trips_are_capped_where_shallnot_stops_waiting_for_the_host() -> None:
    assert CI_ROUND_TRIPS == MAX_HOST_WAITS == 3


def test_the_disclosures_render_as_the_adr_states_them_and_read_back_well_formed() -> None:
    assert CI_SIMULATION.render() == CI_SIMULATION_LINE
    assert ALL_NATIVE_REGATE.render() == ALL_NATIVE_LINE
    for entry in RUN_START_DISCLOSURES:
        reread = parse_entry(entry.render())
        assert reread == entry
        assert reread.well_formed
        assert reread.kind is DeviationKind.PROTOCOL

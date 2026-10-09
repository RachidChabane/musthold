from __future__ import annotations

import json

from hypothesis import given
from hypothesis import strategies as st

from specbench.domain.ci_seed import CI_RUN_PREFIX, RUN_ID_DIGEST_LENGTH, SeedRecord, ci_run_id, ci_seed
from specbench.domain.host_bundle import merge_bundle_manifests, parse_bundle_manifest

HEX_DIGEST = st.text(alphabet="0123456789abcdef", min_size=64, max_size=64)
COMMIT = st.text(alphabet="0123456789abcdef", min_size=40, max_size=40)
RUN_ID = HEX_DIGEST.map(ci_run_id)
RESPONSES = st.frozensets(st.tuples(st.text(alphabet="0123456789abcdef", min_size=4, max_size=8), st.integers(0, 2)))


@given(COMMIT, RUN_ID)
def test_a_seed_is_an_unsigned_64_bit_number_that_is_the_same_every_time(head: str, run_id: str) -> None:
    seed = ci_seed(head, run_id)
    assert 0 <= seed < 2**64
    assert seed == ci_seed(head, run_id)


@given(HEX_DIGEST)
def test_a_run_id_is_the_prefix_and_the_leading_digits_of_the_tree_digest(digest: str) -> None:
    run_id = ci_run_id(digest)
    assert run_id == f"{CI_RUN_PREFIX}-{digest[:RUN_ID_DIGEST_LENGTH]}"
    assert digest.startswith(run_id.removeprefix(f"{CI_RUN_PREFIX}-"))


@given(COMMIT, COMMIT, COMMIT, RUN_ID)
def test_the_record_of_any_head_of_a_tree_carries_the_seed_of_its_agent_commit(
    first_head: str, second_head: str, agent_head: str, run_id: str
) -> None:
    first = SeedRecord.for_tree(first_head, agent_head, run_id)
    second = SeedRecord.for_tree(second_head, agent_head, run_id)
    assert first.seed == second.seed == ci_seed(agent_head, run_id)


@given(COMMIT, COMMIT, RUN_ID)
def test_a_seed_record_reads_back_as_the_fields_it_was_written_from(head: str, agent_head: str, run_id: str) -> None:
    record = SeedRecord.for_tree(head, agent_head, run_id)
    assert json.loads(record.to_json()) == {"headSha": head, "firstRunId": run_id, "seed": record.seed}
    assert record.path.endswith(f"{head}.json")


def manifest_text(head: str, responses: frozenset[tuple[str, int]]) -> str:
    entries = [{"requestId": request_id, "vote": vote} for request_id, vote in sorted(responses)]
    return json.dumps(
        {"module": {"name": "m", "version": "v"}, "headSha": head, "journalDigest": "j", "responses": entries}
    )


@given(RESPONSES, RESPONSES)
def test_the_merged_manifest_lists_exactly_the_responses_of_either_bundle(
    left: frozenset[tuple[str, int]], right: frozenset[tuple[str, int]]
) -> None:
    merged = parse_bundle_manifest(merge_bundle_manifests(manifest_text("a", left), manifest_text("b", right)))
    assert merged.responses == left | right
    assert merged.head_sha == "b"


@given(RESPONSES)
def test_merging_a_bundle_with_itself_changes_nothing(responses: frozenset[tuple[str, int]]) -> None:
    once = merge_bundle_manifests(manifest_text("h", responses), manifest_text("h", responses))
    assert merge_bundle_manifests(once, once) == once

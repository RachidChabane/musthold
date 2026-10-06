"""Descriptive tables of PREREG 11 (everything except cost and failure modes)."""

from __future__ import annotations

from collections import Counter, defaultdict
from collections.abc import Callable, Mapping

from pilot.analysis.agreement import Agreement, cohen_agreement, fleiss_agreement
from pilot.analysis.endpoints import judge_verdicts, matrix_signatures
from pilot.analysis.model import Context
from pilot.analysis.preconditions import (
    BINARY_CATEGORIES,
    binary_truth,
    probe_accuracy,
    rater_binary_labels,
    rater_class_labels,
)
from pilot.analysis.scenarios import (
    GateVariant,
    Scenario,
    TruthKind,
    build_scenarios,
    is_adequate,
    simulate,
)
from pilot.analysis.settings import AnalysisSettings
from pilot.analysis.stats import cohen_kappa, exact_mcnemar_p, wilson
from pilot.analysis.table import Table
from pilot.domain import analysis_constants as k
from pilot.domain.constants import REFACTOR_VARIANT_ID
from pilot.domain.alignment_aggregate import AlignmentVerdict
from pilot.domain.gate import G0, G1, G2, G3, G3_LENIENT, G3_STRICT, GateOutcome, GatePolicy
from pilot.domain.labels import TestClass
from pilot.domain.matrix import PoolMutant, Signature, TestSignature, build_evidence, classify_tests
from pilot.domain.refs import criterion_ref
from pilot.domain.run_records import MutantClassView, MutantMeta

LABELS = tuple(c.value for c in TestClass)
SIGNATURES = tuple(s.value for s in Signature)
VERDICTS = tuple(v.value for v in AlignmentVerdict)
PREDICTED_SIGNATURE = {
    "GOOD": "SPECIFIC",
    "GOOD_BROAD": "OVERBROAD",
    "WEAK": "PARTIAL",
    "VACUOUS": "INERT",
    "MIS_OTHER": "OFF_TARGET",
    "OFFSPEC": "INERT or OFF_TARGET",
}
TABLE_POLICIES = (G0, G1, G2, G3)
FULL_SET_POLICIES: tuple[GatePolicy, ...] = (G0, G1, G2, G3, G3_STRICT, G3_LENIENT)
OUTCOMES = tuple(o.name for o in (GateOutcome.PASS, GateOutcome.ESCALATE, GateOutcome.BLOCK))
NOT_JUDGED = "not judged"
ANSWER_DECLARED, ANSWER_TRUE_OTHER, ANSWER_OTHER, ANSWER_NONE = "declared", "true B", "other", "NONE"
SIX_CLASS = LABELS


def label_of(ctx: Context, nodeid: str) -> str:
    return ctx.labels[nodeid].test_class.value


def pipeline_class(view: MutantClassView) -> str:
    if view.primary_included:
        return "VALID"
    if view.collateral:
        return "COLLATERAL"
    if view.off_target:
        return "OFF_TARGET"
    if view.unattributed:
        return "UNATTRIBUTED"
    if view.equivalent:
        return "EQUIVALENT"
    if view.target is not None and view.verdict_for(view.target) == k.MUTANT_NOT_VIOLATING:
        return "NOT_VIOLATING_TARGET"
    return "UNDETERMINED"


# ------------------------------------------------------------------ confusion matrices


def violation_signatures(ctx: Context) -> dict[str, TestSignature]:
    """The matrix classifier with the 30 violations as the mutant pool (kills from label_matrix.csv)."""
    killed: dict[str, set[str]] = defaultdict(set)
    criterion_of: dict[str, int] = {}
    for nodeid, cells in ctx.truth.label_matrix_rows.items():
        for variant, cell in cells.items():
            if variant == REFACTOR_VARIANT_ID:
                continue
            criterion_of[variant] = int(variant.split("_")[0].removeprefix("PARK-"))
            if cell == "fail":
                killed[variant].add(nodeid)
    pool = [PoolMutant(criterion_of[v], frozenset(killed[v])) for v in sorted(criterion_of)]
    return classify_tests(build_evidence(ctx.declared, pool, []), ctx.declared)


def confusion_label_signature(ctx: Context, signatures: Mapping[str, TestSignature], name: str, title: str) -> Table:
    counts: Counter[tuple[str, str]] = Counter(
        (label_of(ctx, n), s.signature.value) for n, s in signatures.items()
    )
    rows = [[label, PREDICTED_SIGNATURE[label], *(counts[(label, s)] for s in SIGNATURES)] for label in LABELS]
    return Table(name, title, ["label", "predicted", *SIGNATURES], rows)


def judge_arms(ctx: Context) -> list[tuple[str, str]]:
    return sorted({(j, a) for (j, a, _) in ctx.run.alignment})


def confusion_label_verdict(ctx: Context) -> Table:
    rows = []
    for judge, arm in judge_arms(ctx):
        counts: Counter[tuple[str, str]] = Counter()
        for nodeid in ctx.nodeids:
            view = ctx.run.alignment.get((judge, arm, nodeid))
            counts[(label_of(ctx, nodeid), view.verdict.value if view else AlignmentVerdict.UNDETERMINED.value)] += 1
        rows += [[judge, arm, label, *(counts[(label, v)] for v in VERDICTS)] for label in LABELS]
    return Table("confusion_label_x_verdict", "6-class label x judge verdict, per judge and arm",
                 ["judge", "arm", "label", *VERDICTS], rows)


def _answer_type(ctx: Context, nodeid: str, modal_ref: str | None) -> str:
    if modal_ref is None or modal_ref == k.NONE_ANSWER:
        return ANSWER_NONE
    if modal_ref == criterion_ref(ctx.declared[nodeid]):
        return ANSWER_DECLARED
    other = ctx.labels[nodeid].other
    return ANSWER_TRUE_OTHER if other is not None and modal_ref == criterion_ref(other) else ANSWER_OTHER


def confusion_label_answer(ctx: Context) -> Table:
    coverages = ("FULL", "PARTIAL", "WEAK", "NONE")
    columns = [f"{a}|{c}" for a in (ANSWER_DECLARED, ANSWER_TRUE_OTHER, ANSWER_OTHER, ANSWER_NONE) for c in coverages]
    rows = []
    for judge, arm in judge_arms(ctx):
        counts: Counter[tuple[str, str]] = Counter()
        for nodeid in ctx.nodeids:
            view = ctx.run.alignment.get((judge, arm, nodeid))
            if view is None:
                continue
            counts[(label_of(ctx, nodeid), f"{_answer_type(ctx, nodeid, view.modal_ref)}|{view.modal_coverage}")] += 1
        rows += [[judge, arm, label, *(counts[(label, c)] for c in columns)] for label in LABELS]
    return Table("confusion_label_x_answer_type", "6-class label x judge answer type x coverage (modal answer)",
                 ["judge", "arm", "label", *columns], rows)


def single_test_gate(ctx: Context) -> Table:
    singles = [Scenario(ctx.declared[n], frozenset({n}), False) for n in ctx.nodeids]
    rows = []
    for policy in TABLE_POLICIES:
        outcomes = {next(iter(o.scenario.tests)): o.outcome.name
                    for o in simulate(ctx, singles, GateVariant(policy.name, policy))}
        counts = Counter((label_of(ctx, n), outcome) for n, outcome in outcomes.items())
        rows += [[policy.name, label, *(counts[(label, o)] for o in OUTCOMES)] for label in LABELS]
    return Table("confusion_label_x_single_test_gate", "6-class label x single-test gate outcome, G0 to G3",
                 ["policy", "label", *OUTCOMES], rows)


def full_set_outcomes(ctx: Context) -> Table:
    full = [s for s in build_scenarios(ctx) if s.is_full]
    by_policy = {
        p.name: {o.scenario.criterion: o.outcome.name for o in simulate(ctx, full, GateVariant(p.name, p))}
        for p in FULL_SET_POLICIES
    }
    rows = [
        [criterion_ref(c), *(by_policy[p.name][c] for p in FULL_SET_POLICIES)] for c in ctx.criteria
    ]
    return Table("full_set_outcomes", "per-criterion full-set gate outcome under every policy",
                 ["criterion", *(p.name for p in FULL_SET_POLICIES)], rows)


def oracle_x_pipeline(ctx: Context, arm: str) -> Table:
    counts: Counter[tuple[str, str]] = Counter()
    for mutant_id, found in ctx.oracle_class.items():
        if ctx.truth.oracle[mutant_id].arm != arm:
            continue
        view = ctx.run.classes.get(mutant_id)
        counts[(found.kind.value, pipeline_class(view) if view else NOT_JUDGED)] += 1
    oracle_kinds = sorted({o for o, _ in counts})
    pipeline_kinds = sorted({p for _, p in counts})
    rows = [[o, *(counts[(o, p)] for p in pipeline_kinds)] for o in oracle_kinds]
    return Table(f"oracle_x_pipeline_arm_{arm}", f"oracle class x pipeline class, arm {arm}",
                 ["oracle class", *pipeline_kinds], rows)


def cross_check(ctx: Context, verdicts: Mapping[str, AlignmentVerdict], signatures: Mapping[str, TestSignature]) -> Table:
    counts: Counter[tuple[str, str]] = Counter(
        (verdicts[n].value, signatures[n].verdict.value) for n in ctx.nodeids
    )
    rows = [[j, *(counts[(j, m)] for m in VERDICTS)] for j in VERDICTS]
    truth = {n: binary_truth(label_of(ctx, n)) for n in ctx.nodeids}
    judge_right = {n: _correct(verdicts[n], truth[n]) for n in ctx.nodeids}
    matrix_right = {n: _correct(signatures[n].verdict, truth[n]) for n in ctx.nodeids}
    determinate = [n for n in ctx.nodeids if AlignmentVerdict.UNDETERMINED not in (verdicts[n], signatures[n].verdict)]
    agree = sum(1 for n in determinate if verdicts[n] is signatures[n].verdict)
    judge_wrong = [n for n in ctx.nodeids if not judge_right[n]]
    matrix_wrong = [n for n in ctx.nodeids if not matrix_right[n]]
    kappa = cohen_kappa([verdicts[n].value for n in determinate], [signatures[n].verdict.value for n in determinate],
                        [AlignmentVerdict.ALIGNED.value, AlignmentVerdict.MISALIGNED.value]) if determinate else None
    rows += [
        ["Cohen kappa (determinate)", kappa, len(determinate), None],
        ["raw agreement (determinate)", agree, len(determinate), None],
        ["P(matrix correct | judge wrong)", sum(matrix_right[n] for n in judge_wrong), len(judge_wrong), None],
        ["P(judge correct | matrix wrong)", sum(judge_right[n] for n in matrix_wrong), len(matrix_wrong), None],
        ["both wrong", sum(1 for n in ctx.nodeids if not judge_right[n] and not matrix_right[n]), len(ctx.nodeids), None],
        ["matrix silent (UNDETERMINED)", sum(1 for n in ctx.nodeids if signatures[n].verdict is AlignmentVerdict.UNDETERMINED),
         len(ctx.nodeids), None],
    ]
    return Table("judge_matrix_cross_check", "J-SONNET NAME-MASKED (rows) x matrix (columns), with cross-check counts",
                 ["judge verdict / measure", *VERDICTS], rows)


def _correct(verdict: AlignmentVerdict, truth: str) -> bool:
    return (verdict is AlignmentVerdict.ALIGNED) == (truth == k.TRUTH_ALIGNED) and verdict is not AlignmentVerdict.UNDETERMINED


# ------------------------------------------------------------------ limit 2


def limit2_tables(ctx: Context, settings: AnalysisSettings) -> list[Table]:
    return [
        _class_shares(ctx),
        _limit2_measures(ctx),
        _arm_oracle_rates(ctx),
        _yield_table(ctx, "slot", lambda m: m.slot or ""),
        _yield_table(ctx, "generator", lambda m: f"{m.generator_model}/{m.generator_cli}"),
        _audit_agreement(ctx, settings),
    ]


def _class_shares(ctx: Context) -> Table:
    rows: list[list[object]] = []
    for arm in (k.ARM_T, k.ARM_P):
        views = [v for v in ctx.run.classes.values() if v.arm == arm]
        counts = Counter(pipeline_class(v) for v in views)
        rows += [[arm, name, n, n / len(views)] for name, n in sorted(counts.items())]
    for arm in k.ARMS:
        found = [c for m, c in ctx.oracle_class.items() if ctx.truth.oracle[m].arm == arm]
        counts = Counter(c.kind.value for c in found)
        rows += [[f"{arm} (oracle)", name, n, n / len(found)] for name, n in sorted(counts.items())]
    return Table("limit2_class_shares", "class shares per arm (pipeline classes for T and P; oracle classes for every arm)",
                 ["arm", "class", "count", "share"], rows)


def _limit2_measures(ctx: Context) -> Table:
    primary = [v for v in ctx.arm_t_evaluated() if v.primary_included]
    witnessed = {(w.mutant_id, w.criterion) for w in ctx.run.witnesses if w.confirmed}
    backed = sum(1 for v in primary if (v.mutant_id, v.target) in witnessed)
    oracle_valid = [v for v in ctx.arm_t_evaluated() if ctx.oracle_valid_for(v.mutant_id, v.target)]
    discarded = sum(1 for v in oracle_valid if not v.primary_included)
    return Table("limit2_measures", "VALID share backed by an executed witness; false-discard rate",
                 ["measure", "count", "total", "share"], [
        ["VALID mutants with an executed witness for the target", backed, len(primary), backed / len(primary) if primary else None],
        ["oracle-valid mutants not VALID (false discard)", discarded, len(oracle_valid), discarded / len(oracle_valid) if oracle_valid else None],
    ])


def _arm_oracle_rates(ctx: Context) -> Table:
    rows = []
    for arm in (k.ARM_T, k.ARM_O):
        found = [(m, c) for m, c in ctx.oracle_class.items() if ctx.truth.oracle[m].arm == arm]
        valid = sum(1 for _, c in found if c.kind.value == "oracle-valid")
        interval = wilson(valid, len(found))
        rows.append([arm, valid, len(found), valid / len(found) if found else None, interval.lo, interval.hi])
    return Table("limit2_arm_oracle_valid_rate", "oracle-valid share of non-stillborn mutants, arm T vs arm O",
                 ["arm", "oracle-valid", "non-stillborn", "share", "wilson lo", "wilson hi"], rows)


def _yield_table(ctx: Context, name: str, key: Callable[[MutantMeta], str]) -> Table:
    groups: dict[str, list[str]] = defaultdict(list)
    for meta in ctx.run.mutants.values():
        if meta.arm == k.ARM_T:
            groups[key(meta)].append(meta.mutant_id)
    rows = []
    for group, ids in sorted(groups.items()):
        stillborn = sum(1 for m in ids if (p := ctx.run.processing.get(m)) is None or p.stillborn)
        valid = sum(1 for m in ids if (v := ctx.run.classes.get(m)) is not None and v.primary_included)
        oracle = sum(1 for m in ids if ctx.oracle_valid_for(m, ctx.run.mutants[m].target))
        rows.append([group, len(ids), stillborn, valid, oracle])
    return Table(f"limit2_yield_per_{name}", f"arm-T yield per {name}",
                 [name, "candidates", "stillborn", "VALID (primary)", "oracle-valid for target"], rows)


def audit_votes(ctx: Context) -> dict[str, tuple[list[str], int]]:
    """Per audited mutant: A-OPUS, pipeline and oracle 'valid for the declared target' votes and the cluster."""
    sample = ctx.run.audit_sample or {}
    votes: dict[str, tuple[list[str], int]] = {}
    confirmed = {(w.mutant_id, w.criterion) for w in ctx.run.witnesses if w.judge == k.AUDIT_JUDGE and w.confirmed}
    for mutant_id in sample.get("mutant_ids", []):
        target = _audit_target(ctx, mutant_id)
        row = next((r for r in ctx.run.validity if r.mutant_id == mutant_id and r.judge == k.AUDIT_JUDGE
                    and r.status == k.CALL_OK and r.payload), None)
        view = ctx.run.classes.get(mutant_id)
        if target is None or row is None or view is None:
            continue
        requirement = row.payload["requirements"].get(criterion_ref(target), {})  # type: ignore[index]
        audit_valid = (requirement.get("verdict") == "VIOLATED" and (mutant_id, target) in confirmed)
        votes[mutant_id] = (
            [
                _vote(audit_valid),
                _vote(view.verdict_for(target) == k.MUTANT_VALID),
                _vote(ctx.oracle_valid_for(mutant_id, target)),
            ],
            target,
        )
    return votes


def _audit_target(ctx: Context, mutant_id: str) -> int | None:
    planted = ctx.truth.planted.get(mutant_id)
    if planted is not None:
        return planted.declared_target
    meta = ctx.run.mutants.get(mutant_id)
    return meta.target if meta else None


def _vote(valid: bool) -> str:
    return k.MUTANT_VALID if valid else "NOT_VALID"


def _audit_agreement(ctx: Context, settings: AnalysisSettings) -> Table:
    votes = audit_votes(ctx)
    categories = (k.MUTANT_VALID, "NOT_VALID")
    names = ("A-OPUS", "pipeline", "oracle")
    clusters = [target for _, target in votes.values()]
    rows: list[list[object]] = []
    for i, first in enumerate(names):
        for second in names[i + 1:]:
            j = names.index(second)
            a = cohen_agreement(f"{first} vs {second}", [raters[i] for raters, _ in votes.values()],
                                [raters[j] for raters, _ in votes.values()], clusters, categories, settings)
            rows.append(_agreement_row(a))
    fleiss = fleiss_agreement("Fleiss, three raters", [raters for raters, _ in votes.values()], clusters, categories, settings)
    rows.append(_agreement_row(fleiss))
    return Table("limit2_audit_agreement", "audit agreement on 'VALID for the declared target' (A-OPUS, pipeline, oracle)",
                 ["pair", "n", "raw agreement", "kappa", "ci lo", "ci hi"], rows)


def _agreement_row(a: Agreement) -> list[object]:
    return [a.name, a.n, a.raw, a.kappa, a.lo, a.hi]


# ------------------------------------------------------------------ limit 3


def consensus_tests(ctx: Context) -> set[str]:
    """Tests on which the author, R-OAI and R-AUTHOR agree on the 6-class label."""
    raters = rater_class_labels(ctx)
    if len(raters) < 3:
        return set()
    return {n for n in ctx.nodeids if len({raters[r].get(n) for r in raters}) == 1}


def truth_verdict(ctx: Context, nodeid: str) -> AlignmentVerdict:
    return AlignmentVerdict.ALIGNED if ctx.gold_good(nodeid) else AlignmentVerdict.MISALIGNED


def limit3_tables(ctx: Context, settings: AnalysisSettings) -> list[Table]:
    return [_modal_accuracy(ctx), _stability(ctx, settings), _name_anchoring(ctx), _error_overlap(ctx)]


def _modal_accuracy(ctx: Context) -> Table:
    consensus = consensus_tests(ctx)
    rows = []
    for judge, arm in judge_arms(ctx):
        verdicts = judge_verdicts(ctx, judge, arm, False)
        right = {n: verdicts[n] is truth_verdict(ctx, n) for n in ctx.nodeids}
        in_consensus = [n for n in ctx.nodeids if n in consensus]
        rows.append([judge, arm, sum(right.values()), len(right), sum(right[n] for n in in_consensus), len(in_consensus)])
    return Table("limit3_modal_accuracy", "modal accuracy against PRIMARY and CONSENSUS labels (UNDETERMINED counts as wrong)",
                 ["judge", "arm", "PRIMARY correct", "PRIMARY n", "CONSENSUS correct", "CONSENSUS n"], rows)


def _stability(ctx: Context, settings: AnalysisSettings) -> Table:
    rows = []
    for judge, arm in judge_arms(ctx):
        views = [ctx.run.alignment[(judge, arm, n)] for n in ctx.nodeids if (judge, arm, n) in ctx.run.alignment]
        stable = sum(1 for v in views if v.modal_share >= k.MODAL_SHARE_THRESHOLD)
        agreement = _repetition_agreement(ctx, judge, arm, settings)
        rows.append([judge, arm, stable, len(views), stable / len(views) if views else None,
                     agreement.kappa, agreement.lo, agreement.hi, agreement.n])
    return Table("limit3_stability", "share of tests with modal_share >= 0.8; Fleiss kappa over repetitions",
                 ["judge", "arm", "stable", "tests", "share", "fleiss kappa", "ci lo", "ci hi", "complete tests"], rows)


def _repetition_agreement(ctx: Context, judge: str, arm: str, settings: AnalysisSettings) -> Agreement:
    by_test: dict[str, dict[int, str]] = defaultdict(dict)
    for call in ctx.run.alignment_calls:
        if call.judge == judge and call.arm == arm and call.status == k.CALL_OK and call.verifies_ref is not None:
            by_test[call.nodeid][call.rep] = call.verifies_ref
    complete = {n: reps for n, reps in by_test.items() if len(reps) == k.EXPECTED_ALIGNMENT_REPS}
    categories = sorted({ref for reps in complete.values() for ref in reps.values()})
    tests = sorted(complete)
    return fleiss_agreement(
        f"{judge} {arm}", [[complete[n][r] for r in sorted(complete[n])] for n in tests],
        [ctx.labels[n].cites for n in tests], categories, settings,
    )


def _name_anchoring(ctx: Context) -> Table:
    bad = [n for n in ctx.nodeids if ctx.labels[n].test_class in (TestClass.MIS_OTHER, TestClass.OFFSPEC)]
    rows = []
    for judge in sorted({j for j, _ in judge_arms(ctx)}):
        shown = judge_verdicts(ctx, judge, k.ALIGNMENT_ARM_SHOWN, False)
        masked = judge_verdicts(ctx, judge, k.ALIGNMENT_ARM_MASKED, False)
        only_shown = sum(1 for n in bad if shown[n] is AlignmentVerdict.ALIGNED and masked[n] is not AlignmentVerdict.ALIGNED)
        only_masked = sum(1 for n in bad if masked[n] is AlignmentVerdict.ALIGNED and shown[n] is not AlignmentVerdict.ALIGNED)
        rows.append([judge, len(bad), sum(shown[n] is AlignmentVerdict.ALIGNED for n in bad),
                     sum(masked[n] is AlignmentVerdict.ALIGNED for n in bad), only_shown, only_masked,
                     exact_mcnemar_p(only_shown, only_masked)])
    return Table("limit3_name_anchoring", "false ALIGNED on gold MIS_OTHER and OFFSPEC, NAME-SHOWN vs NAME-MASKED (exact McNemar)",
                 ["judge", "tests", "false ALIGNED shown", "false ALIGNED masked", "shown only", "masked only", "p"], rows)


def _error_overlap(ctx: Context) -> Table:
    sonnet = judge_verdicts(ctx, k.JUDGE_SONNET, k.ALIGNMENT_ARM_MASKED, False)
    gpt = judge_verdicts(ctx, k.JUDGE_GPT, k.ALIGNMENT_ARM_MASKED, False)
    counts: Counter[str] = Counter()
    for n in ctx.nodeids:
        wrong_s, wrong_g = sonnet[n] is not truth_verdict(ctx, n), gpt[n] is not truth_verdict(ctx, n)
        counts["both wrong" if wrong_s and wrong_g else "J-SONNET only" if wrong_s else "J-GPT only" if wrong_g else "neither"] += 1
    return Table("limit3_error_overlap", "J-SONNET vs J-GPT error overlap (NAME-MASKED, against PRIMARY labels)",
                 ["pattern", "tests"], [[p, counts[p]] for p in ("both wrong", "J-SONNET only", "J-GPT only", "neither")])


def probe_table(ctx: Context) -> Table:
    rows = []
    for judge in (k.JUDGE_SONNET, k.JUDGE_GPT):
        correct, total = probe_accuracy(ctx, judge)
        rows.append([judge, correct, total])
    return Table("leakage_probe", "leakage probe: tests separated (gold good vs MIS_OTHER and OFFSPEC)",
                 ["judge", "separated", "tests"], rows)


# ------------------------------------------------------------------ contrasts and agreement


def contrasts(ctx: Context, signatures: Mapping[str, TestSignature]) -> Table:
    scenarios = build_scenarios(ctx)
    outcomes = simulate(ctx, scenarios, GateVariant("G3", G3))
    brief = simulate(ctx, scenarios, GateVariant("G3-F_brief", G3, f_brief=True))
    rows: list[list[object]] = []
    for name, criteria in (("poisoned {3, 5, 8}", k.POISONED_CRITERIA), ("clean {1, 2, 4, 9}", k.CLEAN_CRITERIA)):
        adequate = [o for o in outcomes if o.scenario.criterion in criteria and is_adequate(ctx, o.scenario, TruthKind.ORACLE)]
        blocked = sum(1 for o in adequate if o.outcome is GateOutcome.BLOCK)
        interval = wilson(blocked, len(adequate))
        rows.append([f"G3 false-block rate, {name}", blocked, len(adequate), blocked / len(adequate) if adequate else None,
                     interval.lo, interval.hi])
    for policy_name, found in (("G3", outcomes), ("G3-F_brief", brief)):
        inadequate = [o for o in found if not is_adequate(ctx, o.scenario, TruthKind.ORACLE)]
        passed = sum(1 for o in inadequate if o.outcome is GateOutcome.PASS)
        interval = wilson(passed, len(inadequate))
        rows.append([f"{policy_name} false-pass rate", passed, len(inadequate), passed / len(inadequate) if inadequate else None,
                     interval.lo, interval.hi])
    signature_rows = []
    for label in ("GOOD", "GOOD_BROAD"):
        counts = Counter(s.signature.value for n, s in signatures.items() if label_of(ctx, n) == label)
        signature_rows.append([f"{label} signatures", ", ".join(f"{s}={c}" for s, c in sorted(counts.items())), None, None, None, None])
    return Table("contrasts", "G3 on poisoned vs clean criteria; GOOD_BROAD vs GOOD signatures; G3 vs G3-F_brief",
                 ["contrast", "count", "total", "rate", "wilson lo", "wilson hi"], rows + signature_rows)


def inter_rater(ctx: Context, settings: AnalysisSettings) -> Table:
    rows: list[list[object]] = []
    for scheme, raters, categories in (
        ("6-class", rater_class_labels(ctx), SIX_CLASS),
        ("binary", rater_binary_labels(ctx), BINARY_CATEGORIES),
    ):
        names = sorted(raters)
        tests = [n for n in ctx.nodeids if all(n in raters[r] for r in names)]
        clusters = [ctx.labels[n].cites for n in tests]
        for i, first in enumerate(names):
            for second in names[i + 1:]:
                rows.append([scheme, *_agreement_row(cohen_agreement(
                    f"Cohen {first} vs {second}", [raters[first][n] for n in tests],
                    [raters[second][n] for n in tests], clusters, categories, settings))])
        if len(names) >= 3:
            rows.append([scheme, *_agreement_row(fleiss_agreement(
                f"Fleiss {', '.join(names)}", [[raters[r][n] for r in names] for n in tests], clusters, categories, settings))])
    return Table("inter_rater_agreement", "inter-rater agreement, 6-class and binary, bootstrap 95% intervals",
                 ["scheme", "pair", "n", "raw agreement", "kappa", "ci lo", "ci hi"], rows)


def all_signatures(ctx: Context) -> dict[str, TestSignature]:
    return matrix_signatures(ctx)

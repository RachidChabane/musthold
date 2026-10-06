"""Preconditions P1 to P7 (PREREG 10): each returns a status, a detail and the endpoints it affects."""

from __future__ import annotations

import math
from collections.abc import Collection, Mapping, Sequence
from dataclasses import dataclass
from enum import Enum

import numpy as np

from pilot.analysis.model import Context
from pilot.analysis.settings import AnalysisSettings
from pilot.analysis.stats import (
    binomial_greater_p,
    bootstrap_interval,
    fleiss_kappa,
    fleiss_kappa_statistic,
    fleiss_sufficient,
)
from pilot.domain import analysis_constants as k
from pilot.domain.deviations import DeviationEntry, DeviationKind, parse_deviations
from pilot.domain.labels import TestClass

VERIFY_PASS_LINE = "RESULT: PASS"
RATER_COUNT = 3
BINARY_CATEGORIES = (k.TRUTH_ALIGNED, k.TRUTH_NOT_ALIGNED)


class Status(Enum):
    PASS = "pass"
    FAIL = "fail"
    UNEVALUABLE = "unevaluable"


@dataclass(frozen=True)
class Precondition:
    id: str
    description: str
    status: Status
    detail: str
    affects: tuple[str, ...]
    forces_inconclusive: bool = False

    @property
    def holds(self) -> bool:
        return self.status is Status.PASS

    def to_mapping(self) -> dict[str, object]:
        return {
            "id": self.id,
            "description": self.description,
            "status": self.status.value,
            "detail": self.detail,
            "affects": list(self.affects),
            "forces_inconclusive": self.forces_inconclusive,
        }


ALL_ENDPOINTS = k.ENDPOINT_IDS


def p1_ground_truth(ctx: Context) -> Precondition:
    differences = ctx.truth.crosscheck_differences
    verified = VERIFY_PASS_LINE in ctx.truth.verify_log
    if differences is None:
        status, detail = Status.UNEVALUABLE, "the runner cross-check has not been executed"
    elif verified and not differences:
        status, detail = Status.PASS, "label verification passed and the runner cross-check is exact"
    else:
        problems = ([] if verified else ["label verification did not pass"]) + [
            f"{len(differences)} cross-check difference(s)"
        ] * bool(differences)
        status, detail = Status.FAIL, "; ".join(problems)
    return Precondition("P1", "label verification passes and the runner cross-check is exact", status, detail,
                        ALL_ENDPOINTS, forces_inconclusive=True)


def binary_truth(label: str) -> str:
    return k.TRUTH_ALIGNED if TestClass(label).is_gold_good else k.TRUTH_NOT_ALIGNED


def rater_binary_labels(ctx: Context) -> dict[str, dict[str, str]]:
    """Binary alignment truth per rater: AUTHOR (labels file), then each relabel rater."""
    raters: dict[str, dict[str, str]] = {
        "AUTHOR": {n: label.alignment_truth for n, label in ctx.labels.items()}
    }
    for row in ctx.run.relabels:
        raters.setdefault(row.rater, {})[row.nodeid] = binary_truth(row.label)
    return raters


def rater_class_labels(ctx: Context) -> dict[str, dict[str, str]]:
    raters: dict[str, dict[str, str]] = {"AUTHOR": {n: label.test_class.value for n, label in ctx.labels.items()}}
    for row in ctx.run.relabels:
        raters.setdefault(row.rater, {})[row.nodeid] = row.label
    return raters


def fleiss_with_interval(
    ctx: Context, raters: Mapping[str, Mapping[str, str]], categories: Sequence[str], settings: AnalysisSettings
) -> tuple[float, tuple[float, float]]:
    names = sorted(raters)
    tests = [n for n in ctx.nodeids if all(n in raters[r] for r in names)]
    clusters: dict[int, list[list[str]]] = {}
    for nodeid in tests:
        clusters.setdefault(ctx.labels[nodeid].cites, []).append([raters[r][nodeid] for r in names])
    if not tests or len(names) < 2:
        return float("nan"), (float("nan"), float("nan"))
    point = fleiss_kappa([[raters[r][n] for r in names] for n in tests], categories)
    array = np.array([fleiss_sufficient(rows, categories) for rows in clusters.values()])
    interval = bootstrap_interval(array, fleiss_kappa_statistic(len(names)), settings.resamples, settings.seed)
    return point, (interval.lo, interval.hi)


def p2_rater_agreement(ctx: Context, settings: AnalysisSettings) -> Precondition:
    raters = rater_binary_labels(ctx)
    if len(raters) < RATER_COUNT:
        return Precondition("P2", "Fleiss kappa on binary alignment truth", Status.UNEVALUABLE,
                            f"only {sorted(raters)} rated", k.JUDGE_ENDPOINTS + k.CROSS_CHECK_ENDPOINTS)
    point, (lo, _) = fleiss_with_interval(ctx, raters, BINARY_CATEGORIES, settings)
    holds = point >= k.P2_MIN_POINT and lo >= k.P2_MIN_LOWER
    return Precondition(
        "P2", "Fleiss kappa on binary alignment truth (point >= 0.60, lower >= 0.40)",
        Status.PASS if holds else Status.FAIL, f"kappa {point:.3f}, lower {lo:.3f}",
        k.JUDGE_ENDPOINTS + k.CROSS_CHECK_ENDPOINTS,
    )


def modal_guess(rows: Sequence[tuple[str, float]]) -> str:
    """Most frequent guess; ties to the higher mean confidence, then lexicographic order."""
    guesses = sorted({g for g, _ in rows})
    return min(
        guesses,
        key=lambda g: (
            -sum(1 for x, _ in rows if x == g),
            -sum(c for x, c in rows if x == g) / sum(1 for x, _ in rows if x == g),
            g,
        ),
    )


def probe_accuracy(ctx: Context, judge: str) -> tuple[int, int]:
    """Tests where the modal probe guess separates gold good (TARGETED) from MIS_OTHER and OFFSPEC (OTHER)."""
    per_test: dict[str, list[tuple[str, float]]] = {}
    for row in ctx.run.probes:
        if row.judge == judge and row.status == k.CALL_OK and row.guess is not None:
            per_test.setdefault(row.nodeid, []).append((row.guess, row.confidence or 0.0))
    correct = total = 0
    for nodeid, rows in per_test.items():
        label = ctx.labels[nodeid].test_class
        if label.is_gold_good:
            expected = k.PROBE_TARGETED
        elif label in (TestClass.MIS_OTHER, TestClass.OFFSPEC):
            expected = k.PROBE_OTHER
        else:
            continue
        total += 1
        correct += int(modal_guess(rows) == expected)
    return correct, total


def p3_leakage(ctx: Context) -> Precondition:
    correct, total = probe_accuracy(ctx, k.JUDGE_SONNET)
    if total == 0:
        return Precondition("P3", "leakage probe not significant for J-SONNET", Status.UNEVALUABLE,
                            "no probe answers", k.JUDGE_ENDPOINTS + k.CROSS_CHECK_ENDPOINTS)
    p_value = binomial_greater_p(correct, total)
    return Precondition(
        "P3", "leakage probe not significant for J-SONNET (one-sided exact binomial against 0.5, alpha 0.05)",
        Status.PASS if p_value > k.BINOMIAL_ALPHA else Status.FAIL,
        f"{correct}/{total} separated, p = {p_value:.4f}", k.JUDGE_ENDPOINTS + k.CROSS_CHECK_ENDPOINTS,
    )


def pins_of_lock(lock: Mapping[str, object]) -> list[tuple[str, str]]:
    models = lock.get("models", [])
    return [(str(m["model"]), str(m["cli"])) for m in models if isinstance(m, Mapping)]  # type: ignore[index]


def p4_canaries_and_pins(ctx: Context, lock: Mapping[str, object]) -> Precondition:
    pins = pins_of_lock(lock)
    missing = [f"{m}__{c}" for m, c in pins if not any(f.startswith(f"{m}__{c}__") for f in ctx.run.canary_files)]
    manifest_models = _manifest_models(ctx.run.manifest)
    unknown = sorted(manifest_models - {m for m, _ in pins})
    problems = [f"no canary for {m}" for m in missing] + [f"manifest model {m} not in the lock" for m in unknown]
    if not pins:
        problems.append("no pins in the lock")
    return Precondition("P4", "canaries pass and every pin matches", Status.FAIL if problems else Status.PASS,
                        "; ".join(problems) or "all pins have canaries", ALL_ENDPOINTS, forces_inconclusive=True)


def _manifest_models(manifest: Mapping[str, object]) -> set[str]:
    models = manifest.get("models", [])
    names: set[str] = set()
    items = models.values() if isinstance(models, Mapping) else models
    for item in items if isinstance(items, Collection) else []:
        if isinstance(item, str):
            names.add(item)
        elif isinstance(item, Mapping) and "model" in item:
            names.add(str(item["model"]))
    return names


@dataclass(frozen=True)
class MissingRate:
    judge: str
    missing: int
    expected: int

    @property
    def rate(self) -> float:
        return self.missing / self.expected if self.expected else 0.0


def missing_rates(ctx: Context) -> list[MissingRate]:
    rates: list[MissingRate] = []
    evaluated = [m for m, p in ctx.run.processing.items()
                 if not p.stillborn and p.status == k.STATUS_OK and ctx.run.mutants[m].arm in (k.ARM_T, k.ARM_P)]
    for judge in (k.VALIDITY_JUDGE_SONNET, k.VALIDITY_JUDGE_GPT):
        answered = {r.mutant_id for r in ctx.run.validity if r.judge == judge and r.status == k.CALL_OK}
        rates.append(MissingRate(judge, sum(1 for m in evaluated if m not in answered), len(evaluated)))
    for judge in (k.JUDGE_SONNET, k.JUDGE_GPT):
        for arm in (k.ALIGNMENT_ARM_MASKED, k.ALIGNMENT_ARM_SHOWN):
            calls = [c for c in ctx.run.alignment_calls if c.judge == judge and c.arm == arm]
            if not calls and judge != k.JUDGE_SONNET:
                continue
            answered_calls = sum(1 for c in calls if c.status == k.CALL_OK)
            expected = len(ctx.nodeids) * k.EXPECTED_ALIGNMENT_REPS
            rates.append(MissingRate(f"{judge} {arm}", max(expected - answered_calls, 0), expected))
    return rates


def p5_missing_rates(ctx: Context) -> list[Precondition]:
    results = []
    for rate in missing_rates(ctx):
        affects = _judge_endpoints(rate.judge)
        if not affects:
            continue
        holds = rate.rate <= k.P5_MAX_MISSING_RATE
        results.append(Precondition(
            "P5", f"judge missing rate <= 10% ({rate.judge})", Status.PASS if holds else Status.FAIL,
            f"{rate.missing}/{rate.expected} missing ({rate.rate:.1%})", affects))
    return results


def _judge_endpoints(judge: str) -> tuple[str, ...]:
    if judge in (k.VALIDITY_JUDGE_SONNET, k.VALIDITY_JUDGE_GPT):
        return k.GATE_ENDPOINTS + k.VALIDITY_ENDPOINTS + k.MATRIX_ENDPOINTS + k.CROSS_CHECK_ENDPOINTS
    if judge == f"{k.JUDGE_SONNET} {k.ALIGNMENT_ARM_MASKED}":
        return k.GATE_ENDPOINTS + k.JUDGE_ENDPOINTS + k.CROSS_CHECK_ENDPOINTS
    return ()


def _entries_of(ctx: Context, kind: DeviationKind) -> list[DeviationEntry]:
    return [entry for entry in parse_deviations(ctx.run.deviations) if entry.kind is kind]


def _union(entries: Sequence[DeviationEntry]) -> tuple[str, ...]:
    return tuple(e for e in ALL_ENDPOINTS if any(e in entry.endpoints for entry in entries))


def _count(entries: Sequence[DeviationEntry], noun: str) -> str:
    return f"{len(entries)} {noun} entr{'y' if len(entries) == 1 else 'ies'}"


def p6_deviations(ctx: Context) -> Precondition:
    """Protocol entries naming endpoints fail P6 for those endpoints; provenance and quota pauses never do."""
    protocol = _entries_of(ctx, DeviationKind.PROTOCOL)
    touching = [entry for entry in protocol if entry.endpoints]
    malformed = [entry for entry in protocol if not entry.well_formed]
    provenance = _entries_of(ctx, DeviationKind.PROVENANCE)
    parts = [_count(touching, "protocol"), _count(provenance, "provenance")]
    if malformed:
        parts.append(f"{len(malformed)} unparseable line(s) read as protocol entries touching every endpoint")
    return Precondition(
        "P6", "no logged protocol deviation touches a decisional endpoint's inputs",
        Status.FAIL if touching else Status.PASS,
        f"{'; '.join(parts)} logged; see DEVIATIONS.md in the run directory", _union(touching),
    )


def p7_quota(ctx: Context) -> Precondition:
    """A quota-blocked entry makes its endpoints INCONCLUSIVE; a quota pause that resumed does not."""
    blocked = _entries_of(ctx, DeviationKind.QUOTA_BLOCKED)
    pauses = _entries_of(ctx, DeviationKind.QUOTA_PAUSE)
    return Precondition(
        "P7", "no stage blocked by quota exhaustion", Status.FAIL if blocked else Status.PASS,
        f"{_count(blocked, 'quota-blocked')}, {_count(pauses, 'quota-pause')} logged; "
        "see DEVIATIONS.md in the run directory",
        _union(blocked), forces_inconclusive=True,
    )


def evaluate_preconditions(
    ctx: Context, lock: Mapping[str, object], settings: AnalysisSettings
) -> list[Precondition]:
    return [
        p1_ground_truth(ctx),
        p2_rater_agreement(ctx, settings),
        p3_leakage(ctx),
        p4_canaries_and_pins(ctx, lock),
        *p5_missing_rates(ctx),
        p6_deviations(ctx),
        p7_quota(ctx),
    ]


def nan_safe(value: float) -> float | None:
    return None if math.isnan(value) else value

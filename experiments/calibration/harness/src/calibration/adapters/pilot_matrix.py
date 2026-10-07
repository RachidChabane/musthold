"""`PilotMatrix` over `pilot.domain.matrix` of the unpacked snapshot."""

from __future__ import annotations

from collections.abc import Mapping, Sequence

from calibration.adapters.isolated_import import import_pilot_modules
from calibration.application.build_ports import SnapshotSource
from calibration.domain.ports import MatrixSignature

MATRIX_MODULE = "pilot.domain.matrix"


class SnapshotPilotMatrix:
    """Implements `PilotMatrix`: `build_evidence` then `classify_tests` of the snapshot's own matrix module."""

    def __init__(self, snapshot: SnapshotSource) -> None:
        (self._matrix,) = import_pilot_modules(snapshot.pilot_source_root, (MATRIX_MODULE,))

    def classify(
        self,
        nodeids: Sequence[str],
        declared: Mapping[str, int],
        valid_pool: Sequence[tuple[int, frozenset[str]]],
        unattributed: Sequence[frozenset[str]],
    ) -> dict[str, MatrixSignature]:
        pool = [self._matrix.PoolMutant(criterion, killed_by) for criterion, killed_by in valid_pool]
        evidence = self._matrix.build_evidence(nodeids, pool, unattributed)
        signatures = self._matrix.classify_tests(evidence, {nodeid: declared[nodeid] for nodeid in nodeids})
        return {
            nodeid: MatrixSignature(
                nodeid=nodeid,
                declared=int(signature.declared),
                signature=str(signature.signature.value),
                verdict=str(signature.verdict.value),
                none_share=float(signature.none),
            )
            for nodeid, signature in signatures.items()
        }

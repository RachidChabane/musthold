"""What the tickets a gate-host session answered cost: each executor launch joined to its request in the area's host store."""

from __future__ import annotations

from collections.abc import Sequence

from specbench.adapters.execution_counter import intact_stream
from specbench.adapters.gate_session_stream import ticket_launches
from specbench.adapters.host_store import HostStoreRequests
from specbench.domain.clean_room import RoomArea
from specbench.domain.gate import TicketUsage
from specbench.domain.host_requests import request_id_of, request_kind_of


def ticket_usages(lines: Sequence[str], area: RoomArea) -> tuple[TicketUsage, ...]:
    """One usage per executor launch of the stream whose request the area's host store holds."""
    requests = HostStoreRequests(area.state)
    usages: list[TicketUsage] = []
    for launch in ticket_launches(intact_stream(lines)):
        request_id = request_id_of(launch.ticket)
        stored = requests.find(request_id)
        if stored is None:
            continue
        kind = request_kind_of(stored.module.module, stored.kind)
        usages.append(TicketUsage(request_id, kind, stored.module.module, launch.tokens, launch.resolved_model))
    return tuple(usages)

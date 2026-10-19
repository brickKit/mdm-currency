from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Aggregate(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    AGGREGATE_UNSPECIFIED: _ClassVar[Aggregate]
    AGGREGATE_CURRENCY: _ClassVar[Aggregate]
    AGGREGATE_RATE_TYPE: _ClassVar[Aggregate]
    AGGREGATE_EXCHANGE_RATE: _ClassVar[Aggregate]

class ReadState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    READ_STATE_UNSPECIFIED: _ClassVar[ReadState]
    READ_STATE_EXISTS: _ClassVar[ReadState]
    READ_STATE_TOMBSTONE: _ClassVar[ReadState]
    READ_STATE_PURGED: _ClassVar[ReadState]
AGGREGATE_UNSPECIFIED: Aggregate
AGGREGATE_CURRENCY: Aggregate
AGGREGATE_RATE_TYPE: Aggregate
AGGREGATE_EXCHANGE_RATE: Aggregate
READ_STATE_UNSPECIFIED: ReadState
READ_STATE_EXISTS: ReadState
READ_STATE_TOMBSTONE: ReadState
READ_STATE_PURGED: ReadState

class Stamp(_message.Message):
    __slots__ = ("deployment", "instance", "epoch", "positions")
    DEPLOYMENT_FIELD_NUMBER: _ClassVar[int]
    INSTANCE_FIELD_NUMBER: _ClassVar[int]
    EPOCH_FIELD_NUMBER: _ClassVar[int]
    POSITIONS_FIELD_NUMBER: _ClassVar[int]
    deployment: str
    instance: str
    epoch: str
    positions: _containers.RepeatedScalarFieldContainer[int]
    def __init__(self, deployment: _Optional[str] = ..., instance: _Optional[str] = ..., epoch: _Optional[str] = ..., positions: _Optional[_Iterable[int]] = ...) -> None: ...

class Record(_message.Message):
    __slots__ = ("id", "version", "data")
    ID_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    id: str
    version: int
    data: bytes
    def __init__(self, id: _Optional[str] = ..., version: _Optional[int] = ..., data: _Optional[bytes] = ...) -> None: ...

class SnapshotRequest(_message.Message):
    __slots__ = ("aggregate", "cursor", "limit")
    AGGREGATE_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    aggregate: Aggregate
    cursor: str
    limit: int
    def __init__(self, aggregate: _Optional[_Union[Aggregate, str]] = ..., cursor: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class SnapshotResponse(_message.Message):
    __slots__ = ("records", "next_cursor", "stamp")
    RECORDS_FIELD_NUMBER: _ClassVar[int]
    NEXT_CURSOR_FIELD_NUMBER: _ClassVar[int]
    STAMP_FIELD_NUMBER: _ClassVar[int]
    records: _containers.RepeatedCompositeFieldContainer[Record]
    next_cursor: str
    stamp: Stamp
    def __init__(self, records: _Optional[_Iterable[_Union[Record, _Mapping]]] = ..., next_cursor: _Optional[str] = ..., stamp: _Optional[_Union[Stamp, _Mapping]] = ...) -> None: ...

class ReadRequest(_message.Message):
    __slots__ = ("aggregate", "ids")
    AGGREGATE_FIELD_NUMBER: _ClassVar[int]
    IDS_FIELD_NUMBER: _ClassVar[int]
    aggregate: Aggregate
    ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, aggregate: _Optional[_Union[Aggregate, str]] = ..., ids: _Optional[_Iterable[str]] = ...) -> None: ...

class ReadResult(_message.Message):
    __slots__ = ("id", "state", "version", "data")
    ID_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    id: str
    state: ReadState
    version: int
    data: bytes
    def __init__(self, id: _Optional[str] = ..., state: _Optional[_Union[ReadState, str]] = ..., version: _Optional[int] = ..., data: _Optional[bytes] = ...) -> None: ...

class ReadResponse(_message.Message):
    __slots__ = ("results", "stamp")
    RESULTS_FIELD_NUMBER: _ClassVar[int]
    STAMP_FIELD_NUMBER: _ClassVar[int]
    results: _containers.RepeatedCompositeFieldContainer[ReadResult]
    stamp: Stamp
    def __init__(self, results: _Optional[_Iterable[_Union[ReadResult, _Mapping]]] = ..., stamp: _Optional[_Union[Stamp, _Mapping]] = ...) -> None: ...

class EffectiveRateRequest(_message.Message):
    __slots__ = ("rate_type", "date", "from_currency", "to_currency")
    RATE_TYPE_FIELD_NUMBER: _ClassVar[int]
    DATE_FIELD_NUMBER: _ClassVar[int]
    FROM_CURRENCY_FIELD_NUMBER: _ClassVar[int]
    TO_CURRENCY_FIELD_NUMBER: _ClassVar[int]
    rate_type: str
    date: str
    from_currency: str
    to_currency: str
    def __init__(self, rate_type: _Optional[str] = ..., date: _Optional[str] = ..., from_currency: _Optional[str] = ..., to_currency: _Optional[str] = ...) -> None: ...

class EffectiveRateResponse(_message.Message):
    __slots__ = ("id", "version", "rate", "rate_date", "rate_type", "from_currency", "to_currency")
    ID_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    RATE_FIELD_NUMBER: _ClassVar[int]
    RATE_DATE_FIELD_NUMBER: _ClassVar[int]
    RATE_TYPE_FIELD_NUMBER: _ClassVar[int]
    FROM_CURRENCY_FIELD_NUMBER: _ClassVar[int]
    TO_CURRENCY_FIELD_NUMBER: _ClassVar[int]
    id: str
    version: int
    rate: str
    rate_date: str
    rate_type: str
    from_currency: str
    to_currency: str
    def __init__(self, id: _Optional[str] = ..., version: _Optional[int] = ..., rate: _Optional[str] = ..., rate_date: _Optional[str] = ..., rate_type: _Optional[str] = ..., from_currency: _Optional[str] = ..., to_currency: _Optional[str] = ...) -> None: ...
